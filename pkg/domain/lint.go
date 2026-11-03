package domain

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/expr-lang/expr"
)

// LintIssue は claim の意味的な問題。
type LintIssue struct {
	ClaimID string
	Message string
}

// Lint は claim 群の内部整合性を検査する（合議の次に効く機械チェック）。
//   - examples が properties を満たすか（AI が両方書いて食い違っていたら誤読）
//   - 同じ binding・同じ入力に異なる出力を主張する claim 同士の矛盾
//   - properties が式として評価できるか
//   - relations が x と f(...) だけで真偽を返す式か
func Lint(claims []Claim) []LintIssue {
	var issues []LintIssue
	seen := map[string]struct {
		id  string
		out any
	}{}

	for _, c := range claims {
		if err := c.Validate(); err != nil {
			issues = append(issues, LintIssue{c.ID, err.Error()})
			continue
		}
		if c.Kind != KindExample {
			continue
		}
		for _, p := range c.Properties {
			for i, ex := range c.Examples {
				ok, err := EvalProperty(p, ex.In, ex.Out)
				if err != nil {
					issues = append(issues, LintIssue{c.ID, fmt.Sprintf("property %q を評価できない: %v", p, err)})
					break
				}
				if !ok {
					issues = append(issues, LintIssue{c.ID, fmt.Sprintf("examples[%d] (in=%v out=%v) が property %q を満たさない", i, ex.In, ex.Out, p)})
				}
			}
		}
		for _, r := range c.Relations {
			if err := CheckRelation(r); err != nil {
				issues = append(issues, LintIssue{c.ID, fmt.Sprintf("relation %q を評価できない: %v", r, err)})
			}
		}
		for _, ex := range c.Examples {
			key := c.Binding + "|" + canon(ex.In)
			if prev, dup := seen[key]; dup && canon(prev.out) != canon(ex.Out) {
				issues = append(issues, LintIssue{c.ID, fmt.Sprintf("入力 %v の出力が %s と矛盾（%v vs %v）", ex.In, prev.id, prev.out, ex.Out)})
			} else if !dup {
				seen[key] = struct {
					id  string
					out any
				}{c.ID, ex.Out}
			}
		}
	}
	return issues
}

// EvalProperty は expr 式を input / output を束縛して評価する。
func EvalProperty(property string, in, out any) (bool, error) {
	env := map[string]any{
		"input":  normalizeNumber(in),
		"output": normalizeNumber(out),
		"isInteger": func(v any) bool {
			f, ok := toFloat(v)
			return ok && f == math.Trunc(f)
		},
	}
	prog, err := expr.Compile(property, expr.Env(env), expr.AsBool())
	if err != nil {
		return false, err
	}
	v, err := expr.Run(prog, env)
	if err != nil {
		return false, err
	}
	return v.(bool), nil
}

// Equal は example の期待値と実際の値を JSON 正規化して比較する。
func Equal(a, b any) bool { return canon(a) == canon(b) }

func canon(v any) string {
	b, _ := json.Marshal(normalizeNumber(v))
	return string(b)
}

// YAML は 1234 を int、1234.5 を float64 で読むので float64 に揃える。
func normalizeNumber(v any) any {
	switch x := v.(type) {
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case []any:
		out := make([]any, len(x))
		for i := range x {
			out[i] = normalizeNumber(x[i])
		}
		return out
	case map[string]any:
		out := map[string]any{}
		for k := range x {
			out[k] = normalizeNumber(x[k])
		}
		return out
	}
	return v
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	}
	return 0, false
}
