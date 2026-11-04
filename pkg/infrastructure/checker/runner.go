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
	stdout, stderr, err := r.exec(ctx, command, nil)
	return joinOutput(stdout, stderr), err
}

// run は binding 用。成功したら stdout だけを返す（npm の警告など stderr の雑音を出力として読まない）。
// 失敗したら理由を見せるため stdout と stderr の両方を返す（docs/spec/harness.md「抽出結果の試し実行」）。
func (r ShellRunner) run(ctx context.Context, command string, stdin []byte) (string, error) {
	stdout, stderr, err := r.exec(ctx, command, stdin)
	if err != nil {
		return joinOutput(stdout, stderr), err
	}
	return stdout, nil
}

func (r ShellRunner) exec(ctx context.Context, command string, stdin []byte) (stdout, stderr string, err error) {
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
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err = cmd.Run()
	return strings.TrimSpace(out.String()), strings.TrimSpace(errOut.String()), err
}

func joinOutput(stdout, stderr string) string {
	switch {
	case stdout == "":
		return stderr
	case stderr == "":
		return stdout
	default:
		return stdout + "\n" + stderr
	}
}
