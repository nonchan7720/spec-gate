package planfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

func writeFile(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const planMD = `---
title: 月末締め
status: active
claims: [CLOSE-001]
---

# 月末締め

- [ ] 丸めが仕様どおり
  verify: go test ./pkg/round
- [x] 締め日の関数がある
  verify: test -f src/closing.ts
- [ ] verify の無い項目

## 備考
`

func TestLoad_FrontMatterAndItems(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "plans/a.md", planMD)
	p, err := New(root).Load("plans/a.md")
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "月末締め" || p.Status != "active" || len(p.Claims) != 1 || p.Claims[0] != "CLOSE-001" {
		t.Fatalf("front matter: %+v", p)
	}
	if len(p.Items) != 3 {
		t.Fatalf("items = %d, want 3: %+v", len(p.Items), p.Items)
	}
	want := []domain.PlanItem{
		{Text: "丸めが仕様どおり", Verify: "go test ./pkg/round", Line: 9},
		{Text: "締め日の関数がある", Verify: "test -f src/closing.ts", Line: 11, Done: true},
		{Text: "verify の無い項目", Line: 13},
	}
	for i, w := range want {
		if p.Items[i] != w {
			t.Errorf("item %d = %+v, want %+v（Line はファイル上の行番号）", i, p.Items[i], w)
		}
	}
}

func TestLoad_WithoutFrontMatterIsDraft(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "plans/b.md", "- [ ] a\n  verify: true\n")
	p, err := New(root).Load("plans/b.md")
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != "draft" || len(p.Items) != 1 || p.Items[0].Line != 1 {
		t.Fatalf("front matter が無ければ draft、行番号は 1 から: %+v", p)
	}
}

func TestLoad_MutationThreshold(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "plans/a.md", "---\ntitle: a\nmutation: 0.8\n---\n- [ ] x\n  verify: true\n")
	writeFile(t, root, "plans/b.md", "---\ntitle: b\n---\n- [ ] x\n  verify: true\n")
	r := New(root)
	a, err := r.Load("plans/a.md")
	if err != nil {
		t.Fatal(err)
	}
	if a.Mutation == nil || *a.Mutation != 0.8 {
		t.Fatalf("mutation: 0.8 を読む: %v", a.Mutation)
	}
	b, err := r.Load("plans/b.md")
	if err != nil {
		t.Fatal(err)
	}
	if b.Mutation != nil {
		t.Fatalf("mutation が無ければ nil（freeze で score を計算しない）: %v", *b.Mutation)
	}
}

func TestLoad_MutationThresholdInvalid(t *testing.T) {
	for _, v := range []string{"abc", "1.5", "-0.1"} {
		root := t.TempDir()
		writeFile(t, root, "plans/a.md", "---\ntitle: a\nmutation: "+v+"\n---\n- [ ] x\n  verify: true\n")
		if _, err := New(root).Load("plans/a.md"); err == nil {
			t.Errorf("mutation: %s は 0〜1 の数でないのでエラーにする", v)
		}
	}
}

func TestAll_SkipsTemplateAndSorts(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "plans/b.md", "- [ ] b\n")
	writeFile(t, root, "plans/a.md", "- [ ] a\n")
	writeFile(t, root, "plans/TEMPLATE.md", "- [ ] tmpl\n")
	plans, err := New(root).All()
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 2 || plans[0].Path != filepath.Join("plans", "a.md") || plans[1].Path != filepath.Join("plans", "b.md") {
		t.Fatalf("TEMPLATE.md を除き名前順: %+v", plans)
	}
}

func TestFreezeAndLock_RoundTripKeepsOtherPlans(t *testing.T) {
	root := t.TempDir()
	r := New(root)
	if l, err := r.Lock("plans/a.md"); err != nil || l != nil {
		t.Fatalf("ロックファイルが無ければ nil: %v %v", l, err)
	}
	a := domain.PlanLock{Hash: "sha256:a", FrozenAt: "2026-11-04T00:00:00Z", FrozenBy: "alice"}
	b := domain.PlanLock{Hash: "sha256:b", FrozenAt: "2026-11-04T01:00:00Z", FrozenBy: "bob"}
	if err := r.Freeze("plans/a.md", a); err != nil {
		t.Fatal(err)
	}
	if err := r.Freeze("plans/b.md", b); err != nil {
		t.Fatal(err)
	}
	got, err := r.Lock("plans/a.md")
	if err != nil || got == nil || *got != a {
		t.Fatalf("別のプランを凍結しても既存のロックは残る: %+v %v", got, err)
	}
}

func TestLock_BrokenLockFileIsAnError(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".spec-gate/plan.lock.json", "{broken")
	if _, err := New(root).Lock("plans/a.md"); err == nil {
		t.Fatal("壊れたロックファイルは未凍結扱いにせずエラーにする")
	}
}
