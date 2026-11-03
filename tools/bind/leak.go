package main

import (
	"encoding/json"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

func init() {
	// leak: {"code": 対象の内容, "values": 値の配列} → 見つかった値の配列（docs/spec/harness.md「リテラル漏れ検知」）
	register("leak", func(in json.RawMessage) (any, error) {
		var req struct {
			Code   string `json:"code"`
			Values []any  `json:"values"`
		}
		if err := json.Unmarshal(in, &req); err != nil {
			return nil, err
		}
		return domain.LeakedValues(req.Code, req.Values), nil
	})
}
