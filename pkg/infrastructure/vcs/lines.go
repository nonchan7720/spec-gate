package vcs

import (
	"os/exec"
	"strconv"
	"strings"
)

// ChangedLines は `git diff --numstat HEAD` の追加行と削除行の合計（通り方のログ用）。
// git リポジトリでない・HEAD が無いなど数えられないときは 0（hook を落とさない）。
// バイナリファイル（numstat が "-"）と未追跡ファイルは数えない。
func (g Git) ChangedLines() (int, error) {
	cmd := exec.Command("git", "diff", "--numstat", "HEAD")
	cmd.Dir = g.Dir
	out, err := cmd.Output()
	if err != nil {
		return 0, nil
	}
	total := 0
	for l := range strings.SplitSeq(string(out), "\n") {
		f := strings.Fields(l)
		if len(f) < 3 {
			continue
		}
		added, aerr := strconv.Atoi(f[0])
		deleted, derr := strconv.Atoi(f[1])
		if aerr != nil || derr != nil {
			continue
		}
		total += added + deleted
	}
	return total, nil
}
