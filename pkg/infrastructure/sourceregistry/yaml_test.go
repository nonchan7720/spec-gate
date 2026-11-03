package sourceregistry

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

func TestAll_ReadsSources(t *testing.T) {
	root := t.TempDir()
	body := "sources:\n  - type: markdown\n    path: docs/spec.md\n    non_normative: [背景]\n  - type: figma\n    ref: figma://k/n\n    label: 画面\n"
	if err := os.WriteFile(filepath.Join(root, "sources.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := New(root).All()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Type != domain.SourceMarkdown || got[0].RefOf() != "docs/spec.md" || got[0].NonNormative[0] != "背景" || got[1].RefOf() != "figma://k/n" {
		t.Fatalf("sources = %+v", got)
	}
}

func TestAll_MissingFileIsEmpty(t *testing.T) {
	got, err := New(t.TempDir()).All()
	if err != nil || got != nil {
		t.Fatalf("sources.yaml が無ければ空: %v %v", got, err)
	}
}

func TestAll_BrokenFileIsAnError(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "sources.yaml"), []byte("sources: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(root).All(); err == nil {
		t.Fatal("壊れた sources.yaml はエラー")
	}
}
