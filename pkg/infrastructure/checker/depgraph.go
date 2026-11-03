package checker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/internal/textutil"
	"gopkg.in/yaml.v3"
)

// DefaultDepgraphCmd は依存違反の検出を depgraph に任せるときのコマンド。
// depgraph gate は「セッション中に増えた Findings」しか見ないので、既存の違反も通さない
// spec-gate では今のツリーを全件スキャンする。--claude-md ” で CLAUDE.md は書き換えない。
const DefaultDepgraphCmd = "depgraph scan --root . --claude-md ''"

// scanDoneRe は depgraph scan が最後まで走ったときの出力（"scanned 39 files in 771ms"）。
var scanDoneRe = regexp.MustCompile(`(?m)^scanned \d+ files\b`)

// layerViolationRe は depgraph の layer-violation の Finding 1 行。
// 例: "  [error] layer-violation: src/domain -> src/infra (domain must not import infra)"
var layerViolationRe = regexp.MustCompile(`(?m)^\s*\[\w+\] layer-violation: (.+ \((\S+) must not import (\S+)\))\s*$`)

// Depgraph は structure claim を検証する。import の走査は depgraph（検出器）に任せ、
// spec-gate は「図（仕様）と .depgraph.yml（検出器の設定）が一致しているか」を照合する。
//
//  1. claim の各層が .depgraph.yml の layers に同名で存在し、同じディレクトリを指すこと
//  2. claim の層の全順序対 A→B について、図で許可された依存（claim の forbidden を除く）なら
//     depgraph が禁止しておらず、許可されていないなら depgraph が禁止していること
//  3. 一致していれば depgraph で今のツリーを全件スキャンし、claim の層どうしの layer-violation が
//     1 件でもあれば fail（セッション前からある違反も含む）。depgraph が判定できなかった
//     （未インストール・Neo4j に届かない・スキャンが完了しない）ときは unverified
type Depgraph struct {
	root   string
	runner domain.CommandRunner
	cmd    string
}

func NewDepgraph(root string, runner domain.CommandRunner, cmd string) *Depgraph {
	if cmd == "" {
		cmd = DefaultDepgraphCmd
	}
	return &Depgraph{root: root, runner: runner, cmd: cmd}
}

func (Depgraph) Kind() domain.ClaimKind { return domain.KindStructure }

type depgraphConfig struct {
	Layers    map[string][]string `yaml:"layers"`
	Forbidden map[string][]string `yaml:"forbidden"`
}

func (d *Depgraph) Check(ctx context.Context, c domain.Claim, opt domain.CheckOptions) domain.Result {
	res := domain.Result{ID: c.ID, Title: c.Title, Status: domain.StatusPass}
	fail := func(detail string) domain.Result {
		res.Status = domain.StatusFail
		res.Detail = detail
		return res
	}

	useDiagram := c.Allowed == "from-diagram"
	if useDiagram && opt.DiagramEdges == nil {
		return fail("allowed: from-diagram だが図からエッジを取得できなかった")
	}

	cfg, err := d.loadConfig()
	if err != nil {
		return fail(err.Error())
	}
	if issues := mismatches(c, cfg, opt.DiagramEdges, useDiagram); len(issues) > 0 {
		return fail("図（仕様）と .depgraph.yml が一致しない。.depgraph.yml を図に合わせること:\n" + strings.Join(issues, "\n"))
	}

	out, err := d.runner.Run(ctx, d.cmd)
	unverified := func(why string) domain.Result {
		res.Status = domain.StatusUnverified
		res.Detail = "図と .depgraph.yml は一致。" + why + "ため依存違反は未検証:\n$ " + d.cmd + "\n" + textutil.Tail(out, 30)
		return res
	}
	switch {
	case isCommandNotFound(err):
		return unverified("depgraph が見つからない")
	case err != nil:
		// scan の非0は Findings ではなく環境の問題（Neo4j に届かない等）
		return unverified("depgraph が判定できなかった")
	case !scanDoneRe.MatchString(out):
		return unverified("depgraph のスキャン完了を確認できなかった")
	}
	if v := claimViolations(out, c.Layers); len(v) > 0 {
		return fail("層の依存違反（depgraph scan、既存の違反も含む）:\n" + strings.Join(v, "\n"))
	}
	return res
}

// claimViolations は scan の出力から、両端が claim の層である layer-violation を返す。
func claimViolations(out string, layers map[string]string) []string {
	var v []string
	for _, m := range layerViolationRe.FindAllStringSubmatch(out, -1) {
		_, from := layers[m[2]]
		_, to := layers[m[3]]
		if from && to {
			v = append(v, m[1])
		}
	}
	return v
}

func (d *Depgraph) loadConfig() (depgraphConfig, error) {
	var cfg depgraphConfig
	path := filepath.Join(d.root, ".depgraph.yml")
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf(".depgraph.yml が読めない（図の層と禁止 import を書くこと）: %w", err)
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return cfg, fmt.Errorf(".depgraph.yml の解析に失敗: %w", err)
	}
	return cfg, nil
}

func mismatches(c domain.Claim, cfg depgraphConfig, diagram map[string]bool, useDiagram bool) []string {
	var issues []string

	names := make([]string, 0, len(c.Layers))
	for name, dir := range c.Layers {
		names = append(names, name)
		globs, ok := cfg.Layers[name]
		if !ok {
			issues = append(issues, fmt.Sprintf("layers.%s が無い（claim では %s）", name, dir))
			continue
		}
		if !coversDir(globs, dir) {
			issues = append(issues, fmt.Sprintf("layers.%s %v が %s を指していない（%q を含めること）", name, globs, dir, strings.TrimSuffix(cleanDir(dir), "/")+"/**"))
		}
	}
	sort.Strings(names)

	claimForbidden := map[string]bool{}
	for _, f := range c.Forbidden {
		claimForbidden[normEdge(f)] = true
	}
	depForbidden := map[string]bool{}
	for from, tos := range cfg.Forbidden {
		for _, to := range tos {
			depForbidden[from+" -> "+to] = true
		}
	}

	for _, from := range names {
		for _, to := range names {
			if from == to {
				continue
			}
			edge := from + " -> " + to
			allowed := !claimForbidden[edge] && (!useDiagram || diagram[edge])
			switch {
			case allowed && depForbidden[edge]:
				issues = append(issues, fmt.Sprintf("%s は仕様で許可されているが forbidden.%s に %s がある", edge, from, to))
			case !allowed && !depForbidden[edge]:
				issues = append(issues, fmt.Sprintf("%s は仕様で許可されていないが forbidden.%s に %s が無い", edge, from, to))
			}
		}
	}
	return issues
}

func cleanDir(dir string) string {
	return strings.TrimPrefix(filepath.ToSlash(filepath.Clean(dir)), "./")
}

// coversDir は depgraph の glob のどれかが claim の層ディレクトリ全体を指すか。
func coversDir(globs []string, dir string) bool {
	d := cleanDir(dir)
	for _, g := range globs {
		g = strings.TrimSuffix(strings.TrimPrefix(g, "./"), "/")
		if g == d || g == d+"/**" {
			return true
		}
	}
	return false
}

func isCommandNotFound(err error) bool {
	var ee *exec.ExitError
	return errors.As(err, &ee) && ee.ExitCode() == 127
}

func normEdge(e string) string {
	parts := strings.Split(e, "->")
	if len(parts) != 2 {
		return strings.TrimSpace(e)
	}
	return strings.TrimSpace(parts[0]) + " -> " + strings.TrimSpace(parts[1])
}
