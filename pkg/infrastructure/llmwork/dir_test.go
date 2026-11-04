package llmwork

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDir_WriteReadAbsRemove(t *testing.T) {
	root := t.TempDir()
	d := New(root)
	rel := ".spec-gate-llm/extract/a.md/run-1.prompt.md"
	if err := d.Write(rel, "本文"); err != nil {
		t.Fatal(err)
	}
	if got := d.Abs(rel); got != filepath.Join(root, ".spec-gate-llm", "extract", "a.md", "run-1.prompt.md") {
		t.Fatalf("Abs = %s", got)
	}
	s, ok, err := d.Read(rel)
	if err != nil || !ok || s != "本文" {
		t.Fatalf("Read = %q %v %v", s, ok, err)
	}
	if _, ok, err := d.Read(".spec-gate-llm/extract/a.md/run-1.answer.md"); ok || err != nil {
		t.Fatalf("無いファイルは ok=false, err=nil: %v %v", ok, err)
	}
	if err := d.Write(".spec-gate-llm/extract/b.md/run-1.prompt.md", "b"); err != nil {
		t.Fatal(err)
	}
	if err := d.RemoveAll(".spec-gate-llm/extract/a.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".spec-gate-llm", "extract", "a.md")); !os.IsNotExist(err) {
		t.Fatal("a.md が消えていない")
	}
	if _, err := os.Stat(filepath.Join(root, ".spec-gate-llm", "extract", "b.md")); err != nil {
		t.Fatal("b.md を消してはいけない")
	}
	// 空になった親は作業ディレクトリごと片付ける
	if err := d.RemoveAll(".spec-gate-llm/extract/b.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".spec-gate-llm")); !os.IsNotExist(err) {
		t.Fatal("空になった .spec-gate-llm が残っている")
	}
	if err := d.RemoveAll(".spec-gate-llm/reverse"); err != nil {
		t.Fatalf("無いディレクトリの RemoveAll は成功: %v", err)
	}
}

// 作業ディレクトリの外は触らない。
func TestDir_RejectsPathsOutsideWorkDir(t *testing.T) {
	d := New(t.TempDir())
	for _, rel := range []string{"claims/x.yaml", ".spec-gate-llm/../claims/x.yaml", "/etc/passwd", ".spec-gate-llm"} {
		if err := d.Write(rel, "x"); err == nil {
			t.Errorf("Write(%q) は error", rel)
		}
		if err := d.RemoveAll(rel); err == nil {
			t.Errorf("RemoveAll(%q) は error", rel)
		}
	}
}
