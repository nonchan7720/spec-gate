// Package reverser は LLM（既定 `claude -p`）で claim を仕様文に逆変換し、元の仕様と突き合わせる。
package reverser

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/internal/textutil"
	"gopkg.in/yaml.v3"
)

//go:embed prompt_restate.md
var restatePrompt string

//go:embed prompt_compare.md
var comparePrompt string

// Command は stdin で prompt を受け取り stdout に応答を返すコマンド（`sh -c` で実行）を LLM として使う。
// extractor と同じく、Command を差し替えれば他の LLM CLI でも動く。
type Command struct {
	Command string        // 既定は呼び出し側で "claude -p"
	Dir     string        // 作業ディレクトリ。空ならカレント
	Timeout time.Duration // 0 なら無制限（ctx に従う）
}

var _ domain.Reverser = Command{}

// claimView は再生成に渡す claim の内容。元の仕様の位置（source）・承認情報・binding・story などコード側の情報は渡さない。
// 仕様を見ずに claim の主張だけから書き起こさせるため。
type claimView struct {
	Kind       domain.ClaimKind  `yaml:"kind"`
	Title      string            `yaml:"title"`
	Examples   []domain.Example  `yaml:"examples,omitempty"`
	Properties []string          `yaml:"properties,omitempty"`
	Layers     map[string]string `yaml:"layers,omitempty"`
	Allowed    string            `yaml:"allowed,omitempty"`
	Forbidden  []string          `yaml:"forbidden,omitempty"`
	States     []string          `yaml:"states,omitempty"`
	Strings    []string          `yaml:"strings,omitempty"`
}

// Restate は claim の内容だけから仕様文を書き起こさせる。
func (r Command) Restate(ctx context.Context, c domain.Claim) (string, error) {
	view, err := yaml.Marshal(claimView{
		Kind: c.Kind, Title: c.Title, Examples: c.Examples, Properties: c.Properties,
		Layers: c.Layers, Allowed: c.Allowed, Forbidden: c.Forbidden, States: c.States, Strings: c.Strings,
	})
	if err != nil {
		return "", err
	}
	prompt := strings.ReplaceAll(restatePrompt, "{{CLAIM}}", strings.TrimSpace(string(view)))
	out, err := r.run(ctx, prompt)
	if err != nil {
		return "", err
	}
	text := stripFence(out)
	if text == "" {
		return "", errors.New("再生成した仕様文が空")
	}
	return text, nil
}

// Compare は再生成した仕様文と元の仕様を比べさせ、JSON の応答を読む。
func (r Command) Compare(ctx context.Context, restated, original string) (domain.Comparison, error) {
	// 置換結果に再び {{...}} が現れても二重置換しないよう、Replacer で一度に置き換える
	prompt := strings.NewReplacer("{{RESTATED}}", restated, "{{ORIGINAL}}", original).Replace(comparePrompt)
	out, err := r.run(ctx, prompt)
	if err != nil {
		return domain.Comparison{}, err
	}
	return ParseComparison(out)
}

func (r Command) run(ctx context.Context, prompt string) (string, error) {
	if r.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.Timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", r.Command)
	cmd.Dir = r.Dir
	cmd.Stdin = strings.NewReader(prompt)
	// sleep などの子プロセスが stdout を握ったままでも、タイムアウト後に待ち続けない
	cmd.WaitDelay = time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := textutil.Tail(stderr.String(), 10)
		if msg == "" {
			msg = textutil.Tail(stdout.String(), 10)
		}
		return "", fmt.Errorf("%s: %w\n%s", r.Command, err, msg)
	}
	return stdout.String(), nil
}

// stripFence は応答全体が ``` で囲まれていれば中身だけを返す。
func stripFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") || !strings.HasSuffix(s, "```") || len(s) < 6 {
		return s
	}
	body := strings.TrimSuffix(s[3:], "```")
	// 開きフェンスの行（言語名）を落とす
	if _, after, ok := strings.Cut(body, "\n"); ok {
		body = after
	} else {
		body = ""
	}
	return strings.TrimSpace(body)
}

// comparisonJSON は応答の JSON。match は必須なのでポインタで有無を見分ける。
type comparisonJSON struct {
	Match       *bool    `json:"match"`
	Differences []string `json:"differences"`
}

// ParseComparison は LLM の応答から {"match": bool, "differences": [...]} を取り出す。
// ```json フェンスや前後の説明文があっても、最初に読める JSON オブジェクトを使う。
// 読めなければ error（一致とはみなさない）。
func ParseComparison(out string) (domain.Comparison, error) {
	for i := 0; i < len(out); i++ {
		if out[i] != '{' {
			continue
		}
		var v comparisonJSON
		// Decoder は値 1 つだけを読むので、後ろの説明文や閉じフェンスは無視される
		if err := json.NewDecoder(strings.NewReader(out[i:])).Decode(&v); err != nil || v.Match == nil {
			continue
		}
		var diffs []string
		for _, d := range v.Differences {
			if d = strings.TrimSpace(d); d != "" {
				diffs = append(diffs, d)
			}
		}
		// match=true なのに食い違いを挙げる応答は矛盾している。安全側（不一致）に倒す
		return domain.Comparison{Match: *v.Match && len(diffs) == 0, Differences: diffs}, nil
	}
	return domain.Comparison{}, fmt.Errorf(`応答に {"match": bool, "differences": [...]} の JSON が無い:
%s`, textutil.Tail(out, 20))
}
