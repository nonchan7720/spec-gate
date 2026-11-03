package checker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// UI は画面 claim を 2 段階で検証する。
//  1. states: story ファイルに Figma の variant と同名の export があるか（常に実行、決定的）
//  2. strings: --ui 指定時のみ Playwright を生成・実行して文言の表示を確認する。
//     未指定なら unverified として報告し、人間のレビュー対象に残す。
type UI struct {
	root   string
	runner ShellRunner
}

func NewUI(root string, r ShellRunner) *UI { return &UI{root: root, runner: r} }

func (UI) Kind() domain.ClaimKind { return domain.KindUI }

func (u *UI) Check(ctx context.Context, c domain.Claim, opt domain.CheckOptions) domain.Result {
	res := domain.Result{ID: c.ID, Title: c.Title, Status: domain.StatusPass}
	var problems, notes []string

	if c.Story != "" && len(c.States) > 0 {
		b, err := os.ReadFile(filepath.Join(u.root, c.Story))
		if err != nil {
			problems = append(problems, fmt.Sprintf("story が読めない: %v", err))
		} else {
			for _, st := range c.States {
				re := regexp.MustCompile(`(?m)^export\s+(?:const|function)\s+` + regexp.QuoteMeta(st) + `\b`)
				if !re.Match(b) {
					problems = append(problems, fmt.Sprintf("%s に state %q の story がない", c.Story, st))
				}
			}
		}
	}

	if len(c.Strings) > 0 {
		switch {
		case !opt.UI:
			notes = append(notes, fmt.Sprintf("strings %d 件は --ui 未指定のため未検証（Storybook/アプリを起動して `check --ui`）", len(c.Strings)))
		case c.URL == "":
			problems = append(problems, "strings の検証には url が必要")
		default:
			if out, err := u.runPlaywright(ctx, c); err != nil {
				problems = append(problems, "Playwright 失敗:\n    "+indent(truncate(out, 1500)))
			}
		}
	}

	switch {
	case len(problems) > 0:
		res.Status = domain.StatusFail
		res.Detail = strings.Join(append(problems, notes...), "\n")
	case len(notes) > 0:
		res.Status = domain.StatusUnverified
		res.Detail = strings.Join(notes, "\n")
	}
	return res
}

func (u *UI) runPlaywright(ctx context.Context, c domain.Claim) (string, error) {
	dir := filepath.Join(u.root, ".spec-gate", "generated")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("import { test, expect } from '@playwright/test';\n")
	fmt.Fprintf(&b, "test(%q, async ({ page }) => {\n  await page.goto(%q);\n", c.ID, c.URL)
	for _, s := range c.Strings {
		fmt.Fprintf(&b, "  await expect(page.getByText(%q, { exact: true }).first()).toBeVisible();\n", s)
	}
	b.WriteString("});\n")
	spec := filepath.Join(dir, c.ID+".spec.ts")
	if err := os.WriteFile(spec, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	rel, _ := filepath.Rel(u.root, spec)
	return u.runner.Run(ctx, "npx playwright test "+rel+" --reporter=line")
}
