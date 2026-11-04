// Package llmcmd は LLM コマンド（stdin に prompt、stdout に応答）を sh -c で呼ぶ。
package llmcmd

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/internal/textutil"
)

// Shell は command を sh -c で実行する。
type Shell struct{}

var _ domain.LLMRunner = Shell{}

// Run は prompt を stdin に渡して command を実行し、stdout を返す。
func (Shell) Run(ctx context.Context, command, prompt string) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Stdin = strings.NewReader(prompt)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s: %w\n%s", command, err, textutil.Tail(stderr.String(), 20))
	}
	return string(out), nil
}
