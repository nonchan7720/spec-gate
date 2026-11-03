// Package extractor は LLM（claude CLI）で仕様テキストから claim 候補を抽出する。
package extractor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"gopkg.in/yaml.v3"
)

// ClaudeCLI は `claude -p` に prompt を渡し、```yaml ブロックを claim の配列として読む。
// Command を差し替えれば他の LLM CLI でも使える（stdin に prompt、stdout に応答）。
type ClaudeCLI struct {
	Command    string // 既定 "claude -p"
	PromptFile string // 既定 pkg/infrastructure/extractor/prompt.md
}

var yamlBlock = regexp.MustCompile("(?s)```ya?ml\\s*\\n(.*?)```")

func (e ClaudeCLI) Extract(ctx context.Context, ref, excerpt string) ([]domain.Claim, error) {
	tmpl, err := os.ReadFile(e.PromptFile)
	if err != nil {
		return nil, fmt.Errorf("prompt が読めない: %w", err)
	}
	prompt := strings.NewReplacer("{{REF}}", ref, "{{SOURCE}}", excerpt).Replace(string(tmpl))

	cmd := exec.CommandContext(ctx, "sh", "-c", e.Command)
	cmd.Stdin = strings.NewReader(prompt)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", e.Command, err)
	}
	m := yamlBlock.FindSubmatch(out)
	if m == nil {
		return nil, fmt.Errorf("応答に yaml ブロックがない:\n%s", string(out))
	}
	var claims []domain.Claim
	if err := yaml.Unmarshal(m[1], &claims); err != nil {
		return nil, fmt.Errorf("yaml が読めない: %w", err)
	}
	for i := range claims {
		claims[i].Source = domain.Source{Type: claims[i].Source.Type, Ref: ref}
	}
	return claims, nil
}
