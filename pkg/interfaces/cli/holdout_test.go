package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/services"
)

// 組み立て済みの app で、holdout が check / lint に流れ、出力に値が出ないことを確かめる。
func TestApp_HoldoutEndToEnd(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "docs/s.md", "# 仕様\n\n## 恒等\n\n入力をそのまま返す。\n")
	writeFile(t, root, "claims/spec/ID-001.yaml", `id: ID-001
kind: example
title: 恒等
source: { type: markdown, ref: "docs/s.md#恒等" }
binding: sed 's/"in"/"out"/'
examples:
  - { in: 1, out: 1 }
properties:
  - "output < 5000"
`)
	writeFile(t, root, "holdout/ID-001.yaml", "examples:\n  - { in: 31337, out: 31337 }\n  - { in: 2, out: 2 }\n  - { in: 3, out: 4242 }\n")
	a := newApp(root)

	// lint: holdout[0] が property を満たさない。番号だけで値は出さない
	issues, _, err := a.lint.Run()
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "holdout[0]") {
		t.Fatalf("lint は holdout も properties と照合する: %+v", issues)
	}
	assertHidden(t, issues[0].Message)

	if _, err := a.approve.Approve(context.Background(), "ID-001", "test"); err != nil {
		t.Fatal(err)
	}
	rep, err := a.check.Run(context.Background(), services.CheckRequest{NoPlans: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Claims) != 1 || rep.Claims[0].Status != "fail" {
		t.Fatalf("holdout が不一致なら fail: %+v", rep.Claims)
	}
	var human strings.Builder
	writeReport(&human, rep, false)
	code, hook := hookResult(rep)
	if code != 2 {
		t.Fatalf("hook は止める: %d", code)
	}
	for _, out := range []string{human.String(), hook} {
		if !strings.Contains(out, "holdout 3 件中 2 件が不一致（値は表示しない）") {
			t.Fatalf("件数だけを出す:\n%s", out)
		}
		assertHidden(t, out)
	}
}

func assertHidden(t *testing.T, s string) {
	t.Helper()
	for _, v := range []string{"31337", "4242"} {
		if strings.Contains(s, v) {
			t.Fatalf("holdout の値 %s が出ている:\n%s", v, s)
		}
	}
}
