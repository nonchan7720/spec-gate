package vcs

import "testing"

func TestChangedLines_NotARepositoryIsZero(t *testing.T) {
	n, err := Git{Dir: t.TempDir()}.ChangedLines()
	if err != nil || n != 0 {
		t.Fatalf("git でなければ 0: %d %v", n, err)
	}
}

func TestChangedLines_AddedPlusDeleted(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	write(t, dir, "a.txt", "1\n2\n3\n")
	write(t, dir, "bin.dat", "\x00\x01")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "init")

	if n, err := (Git{Dir: dir}).ChangedLines(); err != nil || n != 0 {
		t.Fatalf("変更なしなら 0: %d %v", n, err)
	}

	write(t, dir, "a.txt", "1\nTWO\n3\n4\n") // 2 行追加・1 行削除
	write(t, dir, "s.txt", "x\ny\n")         // ステージした新規ファイル 2 行
	git(t, dir, "add", "s.txt")
	write(t, dir, "bin.dat", "\x00\x02\x03") // バイナリは numstat が "-" なので数えない
	write(t, dir, "untracked.txt", "u\n")    // 未追跡は git diff に出ない
	n, err := Git{Dir: dir}.ChangedLines()
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("git diff --numstat HEAD の追加行と削除行の合計 = 5, got %d", n)
	}
}

func TestChangedLines_NoCommitYetIsZero(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	write(t, dir, "a.txt", "1\n")
	if n, err := (Git{Dir: dir}).ChangedLines(); err != nil || n != 0 {
		t.Fatalf("HEAD が無ければ 0（hook を落とさない）: %d %v", n, err)
	}
}
