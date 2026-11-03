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
type ExampleChecker struct{ runner bindingRunner }

// bindingRunner は stdin つきでコマンドを実行する（テストでは偽物に差し替える）。
type bindingRunner interface {
	run(ctx context.Context, command string, stdin []byte) (string, error)
}

func NewExample(r ShellRunner) *ExampleChecker { return &ExampleChecker{runner: r} }

func (ExampleChecker) Kind() domain.ClaimKind { return domain.KindExample }

func (e *ExampleChecker) Check(ctx context.Context, c domain.Claim, opt domain.CheckOptions) domain.Result {
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
	// examples が通ったときだけ fuzz を回す（不一致があれば既に fail で、binding の呼び出しを増やさない）
	if len(failures) == 0 && c.Fuzz != nil {
		if msg := e.fuzz(ctx, c); msg != "" {
			failures = append(failures, msg)
		}
	}
	if len(opt.Holdout) > 0 {
		results := e.checkHoldout(ctx, c, opt.Holdout)
		for _, r := range results {
			if !r.Pass {
				failures = append(failures, domain.HoldoutSummary(results))
				break
			}
		}
	}
	if len(failures) > 0 {
		res.Status = domain.StatusFail
		res.Detail = strings.Join(failures, "\n")
	}
	return res
}

// fuzz は seed から生成した入力で properties と relations を評価し、最初の反例を返す（無ければ ""）。
// binding の結果は入力ごとに 1 回だけ求めて使い回すので、呼び出しは
// runs ×（1 + relations 中の f(...) の数）回を超えない。
func (e *ExampleChecker) fuzz(ctx context.Context, c domain.Claim) string {
	seed := c.Fuzz.EffectiveSeed(c.ID)
	cache := map[string]any{}
	call := func(x any) (any, error) {
		key, _ := json.Marshal(x)
		if out, ok := cache[string(key)]; ok {
			return out, nil
		}
		in, _ := json.Marshal(map[string]any{"in": x})
		out, err := e.runner.run(ctx, c.Binding, in)
		if err != nil {
			return nil, fmt.Errorf("binding が失敗 (%v)\n    $ %s\n    %s", err, c.Binding, indent(textutil.Tail(out, 12)))
		}
		var got struct {
			Out any `json:"out"`
		}
		if err := json.Unmarshal([]byte(lastJSONLine(out)), &got); err != nil {
			return nil, fmt.Errorf("出力が JSON ではない: %s", truncate(out, 200))
		}
		cache[string(key)] = got.Out
		return got.Out, nil
	}

	inputs := c.Fuzz.Inputs(c.ID)
	for i, x := range inputs {
		where := fmt.Sprintf("fuzz: 反例 x=%v（seed=%d, %d/%d 件目。claim に seed: %d を書けば再現する）", x, seed, i+1, len(inputs), seed)
		out, err := call(x)
		if err != nil {
			return fmt.Sprintf("%s: %v", where, err)
		}
		for _, p := range c.Properties {
			ok, err := domain.EvalProperty(p, x, out)
			if err != nil {
				return fmt.Sprintf("%s: property %q を評価できない: %v", where, p, err)
			}
			if !ok {
				return fmt.Sprintf("%s: 出力 %v が property %q を満たさない", where, out, p)
			}
		}
		for _, r := range c.Relations {
			ok, err := domain.EvalRelation(r, x, call)
			if err != nil {
				return fmt.Sprintf("%s: relation %q を評価できない: %v", where, r, err)
			}
			if !ok {
				return fmt.Sprintf("%s: relation %q を満たさない", where, r)
			}
		}
	}
	return ""
}

// checkHoldout は holdout を binding に流し、1 件ごとの一致だけを返す。
// 期待値は平文で持たないので、実際の出力の正規化 JSON の SHA-256 を out_sha256 と比べ、
// properties は holdout の in と実際の出力で評価する。
// 入力・期待値・実際の値・binding の出力（エラー出力に入力が混ざることがある）は返さない。
// binding が落ちたら残りは流さず不一致として数える（壊れた binding を何度も呼ばない）。
func (e *ExampleChecker) checkHoldout(ctx context.Context, c domain.Claim, holdout []domain.HoldoutExample) []domain.HoldoutResult {
	results := make([]domain.HoldoutResult, len(holdout))
	for i, ex := range holdout {
		in, _ := json.Marshal(map[string]any{"in": ex.In})
		out, err := e.runner.run(ctx, c.Binding, in)
		if err != nil {
			break // results[i:] は Pass: false のまま
		}
		var got struct {
			Out any `json:"out"`
		}
		if json.Unmarshal([]byte(lastJSONLine(out)), &got) != nil {
			continue
		}
		if h, err := domain.OutputHash(got.Out); err != nil || h != ex.OutSHA256 {
			continue
		}
		pass := true
		for _, p := range c.Properties {
			if ok, err := domain.EvalProperty(p, ex.In, got.Out); err != nil || !ok {
				pass = false
				break
			}
		}
		results[i].Pass = pass
	}
	return results
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
