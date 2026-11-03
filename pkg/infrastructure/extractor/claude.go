// Package extractor は LLM（claude CLI）で仕様テキストから claim 候補を抽出する。
package extractor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// ClaudeCLI は `claude -p` に prompt を渡し、```yaml ブロックを claim の配列として読む。
// Command を差し替えれば他の LLM CLI でも使える（stdin に prompt、stdout に応答）。
type ClaudeCLI struct {
	Command    string // 既定 "claude -p"
	PromptFile string // 空なら同梱の prompt.md
}

func (e ClaudeCLI) Extract(ctx context.Context, ref, excerpt string) ([]domain.Claim, error) {
	var p Prompter
	if e.PromptFile != "" {
		tmpl, err := os.ReadFile(e.PromptFile)
		if err != nil {
			return nil, fmt.Errorf("prompt が読めない: %w", err)
		}
		p.Template = string(tmpl)
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", e.Command)
	cmd.Stdin = strings.NewReader(p.Prompt(ref, excerpt))
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", e.Command, err)
	}
	if !yamlBlock.Match(out) {
		return nil, fmt.Errorf("応答に yaml ブロックがない:\n%s", string(out))
	}
	return p.Parse(string(out), ref)
}
