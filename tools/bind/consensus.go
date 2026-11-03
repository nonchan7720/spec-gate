package main

import (
	"encoding/json"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// consensus-hits: run ごとの title の配列（すべて同じ ref の kind: manual）→ 合議でできた束ごとの合意数（初出順）。
func init() {
	register("consensus-hits", func(in json.RawMessage) (any, error) {
		var runs [][]string
		if err := json.Unmarshal(in, &runs); err != nil {
			return nil, err
		}
		perRun := make([][]domain.Claim, 0, len(runs))
		for _, titles := range runs {
			claims := make([]domain.Claim, 0, len(titles))
			for _, t := range titles {
				claims = append(claims, domain.Claim{Kind: domain.KindManual, Title: t, Source: domain.Source{Ref: "spec.md#x"}})
			}
			perRun = append(perRun, claims)
		}
		hits := []int{}
		for _, a := range domain.MergeRuns(perRun) {
			hits = append(hits, a.Hits)
		}
		return hits, nil
	})
}
