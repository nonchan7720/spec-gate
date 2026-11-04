package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
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

// 小さな Go モジュール: cmd/tool → lib/a → lib/b（間接）。lib/c はだれも import しない。
func TestGoDeps(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go が見つからない")
	}
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/m\n\ngo 1.21\n")
	writeFile(t, root, "cmd/tool/main.go", "package main\n\nimport (\n\t\"fmt\"\n\n\t\"example.com/m/lib/a\"\n)\n\nfunc main() { fmt.Println(a.A()) }\n")
	writeFile(t, root, "lib/a/a.go", "package a\n\nimport \"example.com/m/lib/b\"\n\nfunc A() int { return b.B() }\n")
	writeFile(t, root, "lib/b/b.go", "package b\n\nfunc B() int { return 1 }\n")
	writeFile(t, root, "lib/c/c.go", "package c\n")
	writeFile(t, root, "docs/x.md", "# x\n")

	d := NewGoDeps(root)
	cases := []struct {
		in   string
		want []string
	}{
		{"cmd/tool", []string{"cmd/tool", "lib/a", "lib/b"}},         // 直接・間接に import するモジュール内のパッケージ（標準ライブラリは含めない）
		{"./cmd/tool", []string{"cmd/tool", "lib/a", "lib/b"}},       // 先頭の ./ は無視
		{"cmd/tool/main.go", []string{"cmd/tool", "lib/a", "lib/b"}}, // Go ファイルはそのパッケージ
		{"lib/b", []string{"lib/b"}},                                 // import がなければ自分だけ
		{"docs", nil},                                                // Go でないディレクトリ
		{"docs/x.md", nil},                                           // Go でないファイル
		{"missing", nil},                                             // 実在しない
		{"../outside", nil},                                          // リポジトリの外
		{"", nil},
	}
	for _, tc := range cases {
		got := d.Deps(tc.in)
		slices.Sort(got)
		if !slices.Equal(got, tc.want) {
			t.Errorf("Deps(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// 依存を調べられない（壊れたモジュール）ときは何も返さない（元の触るパスは呼び出し側が持つ）。
func TestGoDeps_BrokenModule(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go が見つからない")
	}
	root := t.TempDir()
	writeFile(t, root, "go.mod", "this is not a go.mod\n")
	writeFile(t, root, "lib/a/a.go", "package a\n")
	if got := NewGoDeps(root).Deps("lib/a"); len(got) != 0 {
		t.Fatalf("got %v, want nothing", got)
	}
}

// 同じディレクトリは 1 回の実行の中で 1 度だけ調べる。
func TestGoDeps_CachesPerDirectory(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "lib/a/a.go", "package a\n")
	writeFile(t, root, "lib/a/b.go", "package a\n")
	d := NewGoDeps(root)
	calls := 0
	d.list = func(dir string) ([]string, error) {
		calls++
		return []string{filepath.Join(d.root, "lib", "a")}, nil
	}
	for _, p := range []string{"lib/a", "lib/a/a.go", "./lib/a", "lib/a/b.go"} {
		if got := d.Deps(p); !slices.Equal(got, []string{"lib/a"}) {
			t.Fatalf("Deps(%q) = %v", p, got)
		}
	}
	if calls != 1 {
		t.Fatalf("go list を %d 回呼んだ。ディレクトリごとに 1 回にする", calls)
	}
}
