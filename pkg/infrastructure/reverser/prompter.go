package reverser

import (
	"errors"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"gopkg.in/yaml.v3"
)

// Prompter は逆変換のプロンプトを組み立て、回答を読む（LLM は呼ばない）。
// サブエージェントへの委譲（reverse --emit / --collect）と Command が共有する。
type Prompter struct{}

var _ domain.ReversePrompter = Prompter{}

// RestatePrompt は claim の内容だけを埋めた再生成のプロンプト。元の仕様の位置・binding などは含めない。
func (Prompter) RestatePrompt(c domain.Claim) (string, error) {
	view, err := yaml.Marshal(claimView{
		Kind: c.Kind, Title: c.Title, Examples: c.Examples, Properties: c.Properties,
		Layers: c.Layers, Allowed: c.Allowed, Forbidden: c.Forbidden, States: c.States, Strings: c.Strings,
	})
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(restatePrompt, "{{CLAIM}}", strings.TrimSpace(string(view))), nil
}

// ParseRestate は回答から仕様文を取り出す（全体を囲むフェンスは外す）。空なら error。
func (Prompter) ParseRestate(answer string) (string, error) {
	text := stripFence(answer)
	if text == "" {
		return "", errors.New("再生成した仕様文が空")
	}
	return text, nil
}

// ComparePrompt は再生成した仕様文と元の仕様を埋めた比較のプロンプト。
func (Prompter) ComparePrompt(restated, original string) string {
	// 置換結果に再び {{...}} が現れても二重置換しないよう、Replacer で一度に置き換える
	return strings.NewReplacer("{{RESTATED}}", restated, "{{ORIGINAL}}", original).Replace(comparePrompt)
}

// ParseCompare は {"match": bool, "differences": [...]} を読む。読めなければ error。
func (Prompter) ParseCompare(answer string) (domain.Comparison, error) {
	return ParseComparison(answer)
}
