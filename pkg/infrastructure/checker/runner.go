// Package checker は claim の種類ごとの検証を実装する。
package checker

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"
)

// ShellRunner は `sh -c` でコマンドを実行する。verify 行と binding の両方で使う。
type ShellRunner struct {
	Dir     string
	Timeout time.Duration
}

func (r ShellRunner) Run(ctx context.Context, command string) (string, error) {
	return r.run(ctx, command, nil)
}

func (r ShellRunner) run(ctx context.Context, command string, stdin []byte) (string, error) {
	if r.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.Timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = r.Dir
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}
