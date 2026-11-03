package claimrepo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// h2 / h3 は "2" / "3" の正規化 JSON の SHA-256。
const (
	h2 = "d4735e3a265e16eee03f59718b9b5d03019c07d8b6c51f90da3a666eec13ab35"
	h3 = "4e07408562bedb8b60ce05c1decfe3ad16b72230967de01f640b7e4729b49fce"
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
	writeHoldout(t, root, "ROUND-001.yaml", "examples:\n  - { in: 2.5, out_sha256: "+h2+" }\n  - { in: [1, 2], out_sha256: \""+h3+"\" }\n  - { in: null, out_sha256: "+h2+" }\n")
	exs, err := NewHoldout(root).Holdout("ROUND-001")
	if err != nil {
		t.Fatal(err)
	}
	if len(exs) != 3 || !domain.Equal(exs[0].In, 2.5) || exs[0].OutSHA256 != h2 || exs[1].OutSHA256 != h3 || exs[2].In != nil {
		t.Fatalf("in と out_sha256 を読む: %+v", exs)
	}
}

func TestHoldout_MissingFileIsNil(t *testing.T) {
	exs, err := NewHoldout(t.TempDir()).Holdout("NONE-001")
	if err != nil || exs != nil {
		t.Fatalf("ファイルが無ければ nil, nil: %v %v", exs, err)
	}
}

func TestHoldout_ErrorsHideValues(t *testing.T) {
	cases := map[string]struct{ body, want string }{
		"未知のキー":         {"examples:\n  - { in: 1, out_sha256: " + h2 + " }\nsecret_key: 7777\n", ""},
		"壊れた YAML":      {"examples:\n  - { in: secret-value, out: [\n", ""},
		"型違い":           {"examples: secret-value\n", ""},
		"平文の out":       {"examples:\n  - { in: 1, out_sha256: " + h2 + " }\n  - { in: secret-value, out: 7777 }\n", "examples[1]: out"},
		"out_sha256 なし": {"examples:\n  - { in: secret-value }\n", "out_sha256"},
		"16 進でない":       {"examples:\n  - { in: 1, out_sha256: secret-7777 }\n", "out_sha256"},
		"大文字":           {"examples:\n  - { in: 1, out_sha256: " + strings.ToUpper(h2) + " }\n", "out_sha256"},
		"in なし":         {"examples:\n  - { out_sha256: " + h2 + " }\n", "in"},
	}
	for label, c := range cases {
		t.Run(label, func(t *testing.T) {
			root := t.TempDir()
			writeHoldout(t, root, "A-001.yaml", c.body)
			_, err := NewHoldout(root).Holdout("A-001")
			if err == nil {
				t.Fatal("読めない holdout はエラー")
			}
			for _, v := range []string{"secret", "7777", strings.ToUpper(h2)} {
				if strings.Contains(err.Error(), v) {
					t.Fatalf("エラー文に holdout の中身を出さない: %v", err)
				}
			}
			if !strings.Contains(err.Error(), "holdout/A-001.yaml") || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("どのファイル・どの項目かは出す (%q): %v", c.want, err)
			}
		})
	}
}

func TestHoldout_RejectsUnsafeID(t *testing.T) {
	for _, id := range []string{"../claims/x", "a/b", ""} {
		if _, err := NewHoldout(t.TempDir()).Holdout(id); err == nil {
			t.Fatalf("holdout/ の外を指す id %q は拒否する", id)
		}
		if _, err := NewHoldout(t.TempDir()).Append(id, domain.HoldoutExample{In: 1, OutSHA256: h2}); err == nil {
			t.Fatalf("holdout/ の外を指す id %q には書かない", id)
		}
	}
}

func TestHoldout_All(t *testing.T) {
	root := t.TempDir()
	writeHoldout(t, root, "A-001.yaml", "examples:\n  - { in: 1, out_sha256: "+h2+" }\n")
	writeHoldout(t, root, "B-001.yml", "examples:\n  - { in: 2, out_sha256: "+h2+" }\n")
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

func TestHoldout_AppendCreatesAndAppends(t *testing.T) {
	root := t.TempDir()
	repo := NewHoldout(root)
	path, err := repo.Append("A-001", domain.HoldoutExample{In: map[string]any{"amount": 31337.5}, OutSHA256: h2})
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join("holdout", "A-001.yaml") {
		t.Fatalf("holdout/<id>.yaml に書く: %s", path)
	}
	if _, err := repo.Append("A-001", domain.HoldoutExample{In: "abcd", OutSHA256: h3}); err != nil {
		t.Fatal(err)
	}
	exs, err := repo.Holdout("A-001")
	if err != nil {
		t.Fatal(err)
	}
	if len(exs) != 2 || !domain.Equal(exs[0].In, map[string]any{"amount": 31337.5}) || exs[0].OutSHA256 != h2 ||
		exs[1].In != "abcd" || exs[1].OutSHA256 != h3 {
		t.Fatalf("末尾に足す: %+v", exs)
	}
}

func TestHoldout_AppendKeepsExistingEntriesAndComments(t *testing.T) {
	root := t.TempDir()
	writeHoldout(t, root, "A-001.yaml", "# 人間が書いたメモ\nexamples:\n  - { in: 1, out_sha256: "+h2+" }\n")
	if _, err := NewHoldout(root).Append("A-001", domain.HoldoutExample{In: 2, OutSHA256: h3}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, "holdout", "A-001.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "# 人間が書いたメモ") {
		t.Fatalf("コメントを残す:\n%s", b)
	}
	exs, err := NewHoldout(root).Holdout("A-001")
	if err != nil || len(exs) != 2 || exs[1].OutSHA256 != h3 {
		t.Fatalf("既存の holdout の後ろに足す: %+v %v", exs, err)
	}
}

func TestHoldout_AppendUsesExistingYml(t *testing.T) {
	root := t.TempDir()
	writeHoldout(t, root, "A-001.yml", "examples:\n  - { in: 1, out_sha256: "+h2+" }\n")
	path, err := NewHoldout(root).Append("A-001", domain.HoldoutExample{In: 2, OutSHA256: h3})
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join("holdout", "A-001.yml") {
		t.Fatalf(".yml があればそこに足す（.yaml を別に作ると .yml が読まれなくなる）: %s", path)
	}
}

func TestHoldout_AppendRejectsBrokenFileAndBadHash(t *testing.T) {
	root := t.TempDir()
	body := "examples:\n  - { in: 1, out: 2 }\n"
	writeHoldout(t, root, "A-001.yaml", body)
	if _, err := NewHoldout(root).Append("A-001", domain.HoldoutExample{In: 2, OutSHA256: h3}); err == nil {
		t.Fatal("平文の out が残ったファイルには足さない（先に直す）")
	}
	b, _ := os.ReadFile(filepath.Join(root, "holdout", "A-001.yaml"))
	if string(b) != body {
		t.Fatalf("失敗したら書き換えない:\n%s", b)
	}
	if _, err := NewHoldout(t.TempDir()).Append("B-001", domain.HoldoutExample{In: 2, OutSHA256: "xyz"}); err == nil {
		t.Fatal("形の崩れたハッシュは書かない")
	}
}
