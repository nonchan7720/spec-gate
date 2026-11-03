package vcs

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestChangedFiles_NotARepositoryMeansEverything(t *testing.T) {
	files, err := Git{Dir: t.TempDir()}.ChangedFiles()
	if err != nil || files != nil {
		t.Fatalf("git でなければ nil（= 全件対象）: %v %v", files, err)
	}
}

func TestChangedFiles_ModifiedStagedAndUntracked(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	write(t, dir, "a.txt", "a")
	write(t, dir, "b.txt", "b")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "init")

	write(t, dir, "a.txt", "changed") // 変更（未ステージ）
	write(t, dir, "src/new.ts", "x")  // 未追跡
	write(t, dir, "staged.txt", "s")  // 追加してステージ
	git(t, dir, "add", "staged.txt")
	files, err := Git{Dir: dir}.ChangedFiles()
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(files)
	want := []string{"a.txt", "src/new.ts", "staged.txt"}
	if !slices.Equal(files, want) {
		t.Fatalf("files = %v, want %v", files, want)
	}
}

func TestChangedFiles_NoCommitYetDoesNotDropStagedFiles(t *testing.T) {
	// HEAD が無いと git diff HEAD は失敗する。そこで未追跡だけを返すと、ステージ済みのファイルが
	// 変更一覧から漏れ、--changed がそれに関係する claim を飛ばしてしまう。見逃すより全件を対象にする
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	write(t, dir, "staged.txt", "s")
	git(t, dir, "add", "staged.txt")
	files, err := Git{Dir: dir}.ChangedFiles()
	if err != nil {
		t.Fatal(err)
	}
	if files != nil && !slices.Contains(files, "staged.txt") {
		t.Fatalf("ステージ済みのファイルを落とさない（判定できないなら nil = 全件対象）: %v", files)
	}
}
