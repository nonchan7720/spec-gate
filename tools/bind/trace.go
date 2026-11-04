package main

import (
	"encoding/json"
	"fmt"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// harness.md「通り方のログ」の binding。
func init() {
	// 入力は 1 つの claim の判定履歴（古い順）、出力は {"fails_before_pass", "lines_at_pass"}。pass していなければ null
	register("trace-summary", func(in json.RawMessage) (any, error) {
		var history []domain.TraceEntry
		if err := json.Unmarshal(in, &history); err != nil {
			return nil, fmt.Errorf("入力は判定履歴の配列: %w", err)
		}
		if s := domain.SummarizeTrace(history); s != nil {
			return s, nil
		}
		return nil, nil
	})
}
