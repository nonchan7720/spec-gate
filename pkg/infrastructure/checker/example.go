package checker

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/internal/textutil"
)

// ExampleChecker は binding コマンドに {"in": ...} を渡し、{"out": ...} を期待値と比較する。
// 言語非依存。TypeScript なら tools/call.ts、Go なら go run ./tools/call など、
// 「stdin JSON → stdout JSON」の契約を満たすコマンドなら何でもよい。
type ExampleChecker struct{ runner ShellRunner }

func NewExample(r ShellRunner) *ExampleChecker { return &ExampleChecker{runner: r} }

func (ExampleChecker) Kind() domain.ClaimKind { return domain.KindExample }

func (e *ExampleChecker) Check(ctx context.Context, c domain.Claim, _ domain.CheckOptions) domain.Result {
	res := domain.Result{ID: c.ID, Title: c.Title, Status: domain.StatusPass}
	var failures []string

	for i, ex := range c.Examples {
		in, _ := json.Marshal(map[string]any{"in": ex.In})
		out, err := e.runner.run(ctx, c.Binding, in)
		if err != nil {
			// binding 自体が壊れているなら全 example で同じ失敗になるので 1 回で打ち切る
			failures = append(failures, fmt.Sprintf("examples[%d]: binding が失敗 (%v)。残り %d 件はスキップ\n    $ %s\n    %s",
				i, err, len(c.Examples)-i-1, c.Binding, indent(textutil.Tail(out, 12))))
			break
		}
		var got struct {
			Out any `json:"out"`
		}
		if err := json.Unmarshal([]byte(lastJSONLine(out)), &got); err != nil {
			failures = append(failures, fmt.Sprintf("examples[%d]: 出力が JSON ではない: %s", i, truncate(out, 200)))
			continue
		}
		if !domain.Equal(ex.Out, got.Out) {
			note := ""
			if ex.Note != "" {
				note = " (" + ex.Note + ")"
			}
			failures = append(failures, fmt.Sprintf("examples[%d]%s: in=%v 期待=%v 実際=%v", i, note, ex.In, ex.Out, got.Out))
			continue
		}
		for _, p := range c.Properties {
			ok, err := domain.EvalProperty(p, ex.In, got.Out)
			if err != nil {
				failures = append(failures, fmt.Sprintf("property %q: %v", p, err))
			} else if !ok {
				failures = append(failures, fmt.Sprintf("examples[%d]: 実際の出力 %v が property %q を満たさない", i, got.Out, p))
			}
		}
	}
	if len(failures) > 0 {
		res.Status = domain.StatusFail
		res.Detail = strings.Join(failures, "\n")
	}
	return res
}

// 出力にログが混ざることがあるので、最後の JSON らしき行を取る。
func lastJSONLine(s string) string {
	lines := strings.Split(s, "\n")
	for _, line := range slices.Backward(lines) {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "{") {
			return l
		}
	}
	return s
}

func indent(s string) string { return strings.ReplaceAll(s, "\n", "\n    ") }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
