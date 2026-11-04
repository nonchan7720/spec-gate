package extractor

import (
	_ "embed"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"gopkg.in/yaml.v3"
)

//go:embed prompt.md
var defaultPrompt string

// Prompter は抽出のプロンプトを組み立て、回答を claim として読む（LLM は呼ばない）。
// サブエージェントへの委譲（extract --emit / --collect）と ClaudeCLI が共有する。
type Prompter struct {
	// Template は prompt のテンプレート。空なら同梱の prompt.md
	Template string
}

var _ domain.ExtractPrompter = Prompter{}

var yamlBlock = regexp.MustCompile("(?s)```ya?ml\\s*\\n(.*?)```")

// Prompt は {{REF}} と {{SOURCE}} を埋めたプロンプト。ルールは省略しない。
func (p Prompter) Prompt(ref, excerpt string) string {
	tmpl := p.Template
	if tmpl == "" {
		tmpl = defaultPrompt
	}
	// 置換結果に再び {{...}} が現れても二重置換しないよう、Replacer で一度に置き換える
	return strings.NewReplacer("{{REF}}", ref, "{{SOURCE}}", excerpt).Replace(tmpl)
}

// Parse は回答の ```yaml ブロック（無ければ回答全体）を claim の配列として読み、source.ref を ref にそろえ、
// 例の値を domain.NormalizeExamples でそろえる。
func (Prompter) Parse(answer, ref string) ([]domain.Claim, error) {
	body := answer
	if m := yamlBlock.FindStringSubmatch(answer); m != nil {
		body = m[1]
	}
	var claims []domain.Claim
	if err := yaml.Unmarshal([]byte(body), &claims); err != nil {
		return nil, fmt.Errorf("yaml が読めない: %w", err)
	}
	if len(claims) == 0 {
		return nil, errors.New("回答に claim の yaml が無い")
	}
	for i := range claims {
		claims[i].Source = domain.Source{Type: claims[i].Source.Type, Ref: ref}
		// 構造のある値を JSON の文字列で書いた例を、構造に戻す（docs/spec/harness.md「抽出結果の試し実行」）
		domain.NormalizeExamples(&claims[i])
	}
	return claims, nil
}
