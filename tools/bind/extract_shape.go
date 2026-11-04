package main

import (
	"encoding/json"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// docs/spec/harness.md「manual と例のある claim の同定」「抽出結果の試し実行」「まとめ承認」「manual の手がかり」の binding。
func init() {
	// consensus-kinds: run ごとの {"kind", "title", "in"?, "out"?} の配列 → 束ごとの {"kind", "hits"}
	register("consensus-kinds", func(in json.RawMessage) (any, error) {
		var runs [][]struct {
			Kind  domain.ClaimKind `json:"kind"`
			Title string           `json:"title"`
			In    any              `json:"in"`
			Out   any              `json:"out"`
		}
		if err := json.Unmarshal(in, &runs); err != nil {
			return nil, err
		}
		perRun := make([][]domain.Claim, 0, len(runs))
		for _, r := range runs {
			var cs []domain.Claim
			for _, c := range r {
				cl := domain.Claim{Kind: c.Kind, Title: c.Title, Source: domain.Source{Ref: "spec.md#x"}}
				if c.Kind == domain.KindExample {
					cl.Examples = []domain.Example{{In: c.In, Out: c.Out}}
				}
				cs = append(cs, cl)
			}
			perRun = append(perRun, cs)
		}
		out := []map[string]any{}
		for _, a := range domain.MergeRuns(perRun) {
			out = append(out, map[string]any{"kind": a.Claim.Kind, "hits": a.Hits})
		}
		return out, nil
	})

	// example-value: 回答に書かれた例の値 → そろえた値
	register("example-value", func(in json.RawMessage) (any, error) {
		var v any
		if err := json.Unmarshal(in, &v); err != nil {
			return nil, err
		}
		return domain.NormalizeExampleValue(v), nil
	})

	// dry-run-summary: {"total", "mismatch", "error"} → `# dry-run:` に書く文字列
	register("dry-run-summary", func(in json.RawMessage) (any, error) {
		var d struct {
			Total    int    `json:"total"`
			Mismatch int    `json:"mismatch"`
			Error    string `json:"error"`
		}
		if err := json.Unmarshal(in, &d); err != nil {
			return nil, err
		}
		return domain.DryRun{Total: d.Total, Mismatch: d.Mismatch, Error: d.Error}.Summary(), nil
	})

	// review-order: 候補 {"name", "kind", "confidence", "dry_run"} の配列 → 並べたあとの name の配列
	register("review-order", func(in json.RawMessage) (any, error) {
		var cs []pendingIn
		if err := json.Unmarshal(in, &cs); err != nil {
			return nil, err
		}
		ps := make([]domain.PendingClaim, 0, len(cs))
		for _, c := range cs {
			ps = append(ps, c.pending())
		}
		domain.SortForReview(ps)
		names := make([]string, 0, len(ps))
		for _, p := range ps {
			names = append(names, p.Name)
		}
		return names, nil
	})

	// bulk-eligible: 候補 {"kind", "confidence", "dry_run", "min_agree"} → まとめ承認の対象か
	register("bulk-eligible", func(in json.RawMessage) (any, error) {
		var c struct {
			pendingIn
			MinAgree int `json:"min_agree"`
		}
		if err := json.Unmarshal(in, &c); err != nil {
			return nil, err
		}
		return domain.BulkEligible(c.pending(), c.MinAgree), nil
	})

	// manual-rate: claim の kind の配列 → manual の割合
	register("manual-rate", func(in json.RawMessage) (any, error) {
		var kinds []domain.ClaimKind
		if err := json.Unmarshal(in, &kinds); err != nil {
			return nil, err
		}
		return domain.ManualRate(kinds), nil
	})
}

type pendingIn struct {
	Name       string           `json:"name"`
	Kind       domain.ClaimKind `json:"kind"`
	Confidence string           `json:"confidence"`
	DryRun     string           `json:"dry_run"`
}

// pending は候補を作る。kind: example は binding がある（試し実行の対象）ものとする。
func (p pendingIn) pending() domain.PendingClaim {
	c := domain.Claim{ID: p.Name, Kind: p.Kind}
	if p.Kind == domain.KindExample {
		c.Binding = "go run ./tools/bind x"
	}
	return domain.PendingClaim{Name: p.Name, Confidence: p.Confidence, DryRun: p.DryRun, Claim: c}
}
