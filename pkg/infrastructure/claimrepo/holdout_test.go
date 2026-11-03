package claimrepo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

func writeHoldout(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, "holdout")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestHoldout_ReadsExamples(t *testing.T) {
	root := t.TempDir()
	writeHoldout(t, root, "ROUND-001.yaml", "examples:\n  - { in: 2.5, out: 2 }\n  - { in: [1, 2], out: 3 }\n")
	exs, err := NewHoldout(root).Holdout("ROUND-001")
	if err != nil {
		t.Fatal(err)
	}
	if len(exs) != 2 || !domain.Equal(exs[0].In, 2.5) || !domain.Equal(exs[1].Out, 3) {
		t.Fatalf("examples を読む: %+v", exs)
	}
}

func TestHoldout_MissingFileIsNil(t *testing.T) {
	exs, err := NewHoldout(t.TempDir()).Holdout("NONE-001")
	if err != nil || exs != nil {
		t.Fatalf("ファイルが無ければ nil, nil: %v %v", exs, err)
	}
}

func TestHoldout_ErrorsHideValues(t *testing.T) {
	cases := map[string]string{
		"未知のキー":    "examples:\n  - { in: 1, out: 1 }\nsecret_key: 7777\n",
		"壊れた YAML": "examples:\n  - { in: secret-value, out: [\n",
		"型違い":      "examples: secret-value\n",
	}
	for label, body := range cases {
		t.Run(label, func(t *testing.T) {
			root := t.TempDir()
			writeHoldout(t, root, "A-001.yaml", body)
			_, err := NewHoldout(root).Holdout("A-001")
			if err == nil {
				t.Fatal("読めない holdout はエラー")
			}
			for _, v := range []string{"secret", "7777"} {
				if strings.Contains(err.Error(), v) {
					t.Fatalf("エラー文に holdout の中身を出さない: %v", err)
				}
			}
			if !strings.Contains(err.Error(), "holdout/A-001.yaml") {
				t.Fatalf("どのファイルかは出す: %v", err)
			}
		})
	}
}

func TestHoldout_RejectsUnsafeID(t *testing.T) {
	for _, id := range []string{"../claims/x", "a/b", ""} {
		if _, err := NewHoldout(t.TempDir()).Holdout(id); err == nil {
			t.Fatalf("holdout/ の外を指す id %q は拒否する", id)
		}
	}
}

func TestHoldout_All(t *testing.T) {
	root := t.TempDir()
	writeHoldout(t, root, "A-001.yaml", "examples:\n  - { in: 1, out: 1 }\n")
	writeHoldout(t, root, "B-001.yml", "examples:\n  - { in: 2, out: 2 }\n")
	writeHoldout(t, root, "README.md", "not yaml")
	all, err := NewHoldout(root).All()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || len(all["A-001"]) != 1 || len(all["B-001"]) != 1 {
		t.Fatalf("yaml だけを claim ID ごとに読む: %+v", all)
	}

	none, err := NewHoldout(t.TempDir()).All()
	if err != nil || len(none) != 0 {
		t.Fatalf("holdout/ が無ければ空: %v %v", none, err)
	}
}
