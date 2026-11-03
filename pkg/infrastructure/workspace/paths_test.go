package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathsExists(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src", "payroll"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "payroll", "round.ts"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// リポジトリの外にあるファイル（../outside.ts で指せる）
	outside := filepath.Join(filepath.Dir(root), "outside.ts")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(outside) })

	p := Paths{Root: root}
	cases := map[string]bool{
		"src/payroll/round.ts":   true,  // ファイル
		"src/payroll":            true,  // ディレクトリ
		"./src/payroll/round.ts": true,  // 先頭の ./ は無視
		"src/payroll/util.ts":    false, // 実在しない
		"roundYen":               false, // パスでない語
		"../outside.ts":          false, // リポジトリの外
		outside:                  false, // 絶対パス
		"":                       false,
	}
	for in, want := range cases {
		if got := p.Exists(in); got != want {
			t.Errorf("Exists(%q) = %v, want %v", in, got, want)
		}
	}
}
