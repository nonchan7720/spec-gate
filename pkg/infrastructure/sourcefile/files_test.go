package sourcefile

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func write(t *testing.T, root, rel, body string, mode os.FileMode) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

func TestExpand_FilesAndDirectoriesWithoutTestsOrMissing(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{
		"src/payroll/round.ts",
		"src/payroll/round.test.ts",
		"src/payroll/round.spec.ts",
		"src/app/handler/x.ts",
		"src/app/handler/x_test.go",
		"src/app/handler/y.go",
		"src/app/README.md",
		"src/app/node_modules/dep/index.js",
		"src/app/.hidden/z.ts",
		"tools/bind/main.go",
	} {
		write(t, root, f, "x\n", 0o644)
	}
	got, err := New(root).Expand([]string{
		"src/payroll/round.ts", "src/payroll/round.test.ts", // テストファイルは除く
		"./src/app",            // ディレクトリは配下のソースファイル
		"./tools/bind",         // binding の ./ 付きパス
		"src/missing.ts",       // 実在しないパスは除く
		"../outside.go",        // ルートの外は触らない
		"src/payroll/round.ts", // 重複は 1 つに
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"src/app/handler/x.ts", "src/app/handler/y.go", "src/payroll/round.ts", "tools/bind/main.go"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestReadWrite_KeepsPermission(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a/run.sh", "echo 1\n", 0o755)
	f := New(root)
	s, err := f.Read("a/run.sh")
	if err != nil || s != "echo 1\n" {
		t.Fatalf("read: %q %v", s, err)
	}
	if err := f.Write("a/run.sh", "echo 2\n"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(root, "a/run.sh"))
	st, _ := os.Stat(filepath.Join(root, "a/run.sh"))
	if string(b) != "echo 2\n" || st.Mode().Perm() != 0o755 {
		t.Fatalf("中身を置き換え、パーミッションは保つ: %q %v", b, st.Mode())
	}
}
