package domain

import (
	"encoding/json"
	"fmt"
	"strings"
)

// 抽出結果の試し実行（docs/spec/harness.md「抽出結果の試し実行」）。

// NormalizeExampleValue は回答に書かれた例の値をそろえる。LLM は構造のある値を JSON の文字列にして書くことがあるので、
// 前後の空白を除いた中身が {…} か […] で JSON として読める文字列なら、読んだ値に置き換える。
// 数値・真偽値に読める文字列、JSON として読めない文字列、入れ子の中の文字列は変えない。
func NormalizeExampleValue(v any) any {
	s, ok := v.(string)
	if !ok {
		return v
	}
	t := strings.TrimSpace(s)
	if len(t) < 2 || (t[0] != '{' || t[len(t)-1] != '}') && (t[0] != '[' || t[len(t)-1] != ']') {
		return v
	}
	var out any
	if err := json.Unmarshal([]byte(t), &out); err != nil {
		return v
	}
	return out
}

// NormalizeExamples は claim の examples の in / out を NormalizeExampleValue でそろえる。
func NormalizeExamples(c *Claim) {
	for i := range c.Examples {
		c.Examples[i].In = NormalizeExampleValue(c.Examples[i].In)
		c.Examples[i].Out = NormalizeExampleValue(c.Examples[i].Out)
	}
}

// DryRun は試し実行の結果。
type DryRun struct {
	// Total は流した例の数
	Total int
	// Mismatch は期待と違った例の数
	Mismatch int
	// Error は binding を起動できない・終了コードが 0 でない・出力が JSON でないときの理由。空なら動いた
	Error string
}

// DryRunState は試し実行の結果の分類。
type DryRunState string

const (
	DryRunPass  DryRunState = "pass"
	DryRunFail  DryRunState = "fail"
	DryRunError DryRunState = "error"
	DryRunNone  DryRunState = "none"
)

// dryRunErrorMax は `# dry-run: error:` に書く理由の最大文字数。
const dryRunErrorMax = 120

// State は結果を分類する。
func (d DryRun) State() DryRunState {
	switch {
	case d.Error != "":
		return DryRunError
	case d.Total == 0:
		return DryRunNone
	case d.Mismatch > 0:
		return DryRunFail
	default:
		return DryRunPass
	}
}

// Summary は候補ファイルの `# dry-run:` に書く 1 行。
func (d DryRun) Summary() string {
	switch d.State() {
	case DryRunError:
		line, _, _ := strings.Cut(strings.TrimSpace(d.Error), "\n")
		if r := []rune(line); len(r) > dryRunErrorMax {
			line = string(r[:dryRunErrorMax])
		}
		return "error: " + line
	case DryRunFail:
		return fmt.Sprintf("fail: %d 件中 %d 件が期待と違う", d.Total, d.Mismatch)
	case DryRunNone:
		return "none"
	default:
		return "pass"
	}
}

// DryRunStateOf はヘッダの `# dry-run:` の値を分類する。空や読めない値は none。
func DryRunStateOf(summary string) DryRunState {
	head, _, _ := strings.Cut(strings.TrimSpace(summary), ":")
	switch DryRunState(head) {
	case DryRunPass, DryRunFail, DryRunError:
		return DryRunState(head)
	default:
		return DryRunNone
	}
}

// NeedsDryRun は試し実行の対象か（binding のある kind: example）。
func (c Claim) NeedsDryRun() bool { return c.Kind == KindExample && c.Binding != "" }
