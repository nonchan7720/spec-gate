package main

import (
	"encoding/json"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

func init() {
	// holdout-summary: holdout の判定結果の配列 → レポートに出す 1 行（docs/spec/harness.md「holdout examples」）。
	// 入力に in / out / got があっても読まない（値を出さないことを binding でも担保する）
	register("holdout-summary", func(in json.RawMessage) (any, error) {
		var results []domain.HoldoutResult
		if err := json.Unmarshal(in, &results); err != nil {
			return nil, err
		}
		return domain.HoldoutSummary(results), nil
	})
}
