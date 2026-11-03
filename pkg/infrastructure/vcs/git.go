// Package vcs は作業ツリーの変更ファイルを git から取る。
package vcs

import (
	"os/exec"
	"strings"
)

type Git struct{ Dir string }

// ChangedFiles は HEAD との差分 + 未追跡ファイル。git リポジトリでなければ nil（= 全件対象）。
func (g Git) ChangedFiles() ([]string, error) {
	run := func(args ...string) []string {
		cmd := exec.Command("git", args...)
		cmd.Dir = g.Dir
		out, err := cmd.Output()
		if err != nil {
			return nil
		}
		var files []string
		for l := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
			if l != "" {
				files = append(files, l)
			}
		}
		return files
	}
	if err := exec.Command("git", "-C", g.Dir, "rev-parse").Run(); err != nil {
		return nil, nil
	}
	files := run("diff", "--name-only", "HEAD")
	files = append(files, run("ls-files", "--others", "--exclude-standard")...)
	return files, nil
}
