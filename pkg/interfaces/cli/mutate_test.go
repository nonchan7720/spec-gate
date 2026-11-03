package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

func TestPrintMutation_NoMutantsPrintsNoScore(t *testing.T) {
	var b bytes.Buffer
	th := 0.5
	ok := printMutation(&b, domain.Plan{Path: "plans/a.md", Mutation: &th}, domain.MutationResult{})
	if ok {
		t.Fatal("閾値があるのに変異を作れなければ失敗にする")
	}
	if strings.Contains(b.String(), "score 0") || !strings.Contains(b.String(), "score なし") {
		t.Fatalf("score は出さない: %q", b.String())
	}
	if !printMutation(&b, domain.Plan{Path: "plans/a.md"}, domain.MutationResult{}) {
		t.Fatal("閾値が無ければ失敗にしない")
	}
}

func TestPrintMutation_ScoreSurvivorsAndThreshold(t *testing.T) {
	var b bytes.Buffer
	th := 0.8
	res := domain.MutationResult{Candidates: 5, Total: 3, Killed: 2, Survivors: []domain.FileMutant{
		{File: "src/calc.js", Line: 4, Before: "  return x > 0", After: "  return x >= 0"},
	}}
	if printMutation(&b, domain.Plan{Path: "plans/a.md", Mutation: &th}, res) {
		t.Fatal("0.67 < 0.8 は失敗")
	}
	for _, want := range []string{"0.67", "2/3", "5 件から", "src/calc.js:4", "return x > 0  →  return x >= 0", "閾値 0.8 未満"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("%q が無い:\n%s", want, b.String())
		}
	}
}

func writeProject(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

const calcJS = "export function add(a, b) {\n  return a + b\n}\n\nexport function positive(x) {\n  return x > 0\n}\n"

func mutateProject(t *testing.T, verify string) string {
	t.Helper()
	root := t.TempDir()
	writeProject(t, root, map[string]string{
		"claims/spec/CALC-001.yaml": "id: CALC-001\nkind: example\ntitle: 足し算\nsource:\n  type: markdown\n  ref: docs/spec/calc.md#足し算\nbinding: node tools/call.js src/calc.js add\nexamples:\n  - in: 1\n    out: 1\n",
		"src/calc.js":               calcJS,
		"src/calc.test.js":          "a + b\n",
		"plans/a.md":                "---\ntitle: calc\nmutation: 0.5\nclaims: [CALC-001]\n---\n- [ ] 足し算\n  verify: " + verify + "\n",
	})
	return root
}

// 実ファイルと sh で verify を走らせ、score と生き残りを出し、ファイルを元に戻す。
func TestMutate_EndToEndRestoresRealFiles(t *testing.T) {
	// "a + b" と "x > 0" が残っていることを確かめる verify（a - b・x >= 0・x < 0 を検出する）
	root := mutateProject(t, `grep -q "a + b" src/calc.js && grep -q "x > 0" src/calc.js`)
	a := newApp(root)
	res, err := a.mutation.Run(context.Background(), "plans/a.md", 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 3 || res.Killed != 3 {
		t.Fatalf("3 件すべて検出: %+v", res)
	}
	b, _ := os.ReadFile(filepath.Join(root, "src/calc.js"))
	if string(b) != calcJS {
		t.Fatalf("元に戻す: %q", b)
	}
	if t2, _ := os.ReadFile(filepath.Join(root, "src/calc.test.js")); string(t2) != "a + b\n" {
		t.Fatal("テストファイルには変異を入れない")
	}
	if entries, _ := filepath.Glob(filepath.Join(root, "src", ".spec-gate-mutate-*")); len(entries) != 0 {
		t.Fatalf("一時ファイルを残さない: %v", entries)
	}
}

// 凍結（freeze）は閾値未満なら拒否し、ロックを書かず、ファイルを戻す。
func TestFreeze_EndToEndRefusesWeakVerify(t *testing.T) {
	root := mutateProject(t, "test -f src/calc.js") // 何も検出しない verify
	a := newApp(root)
	_, res, err := a.plan.Freeze(context.Background(), "plans/a.md", "tester")
	if err == nil || !strings.Contains(err.Error(), "凍結しない") {
		t.Fatalf("score 0 < 0.5 は拒否: %v", err)
	}
	if res == nil || res.Killed != 0 || res.Total != 3 {
		t.Fatalf("結果: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(root, ".spec-gate", "plan.lock.json")); !os.IsNotExist(err) {
		t.Fatal("拒否したらロックを書かない")
	}
	if b, _ := os.ReadFile(filepath.Join(root, "src/calc.js")); string(b) != calcJS {
		t.Fatalf("元に戻す: %q", b)
	}
}
