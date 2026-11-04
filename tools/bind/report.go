package main

import (
	"encoding/json"
	"fmt"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// harness.md「CI 用の出力と指標」「役割別レポート」の binding。
func init() {
	// 入力は status の配列、出力は unverified 率
	register("unverified-rate", func(in json.RawMessage) (any, error) {
		var statuses []domain.Status
		if err := json.Unmarshal(in, &statuses); err != nil {
			return nil, fmt.Errorf("入力は status の配列: %w", err)
		}
		return domain.UnverifiedRate(statuses), nil
	})
}
