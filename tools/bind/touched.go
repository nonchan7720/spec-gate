package main

import (
	"encoding/json"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// touched: {"touches": [パス...], "changed": 変更ファイル} → その claim が対象か（真偽）。
// touches は実在確認を済ませたパスとして扱う。
func init() {
	register("touched", func(in json.RawMessage) (any, error) {
		var req struct {
			Touches []string `json:"touches"`
			Changed string   `json:"changed"`
		}
		if err := json.Unmarshal(in, &req); err != nil {
			return nil, err
		}
		return domain.IsTouched(req.Touches, req.Changed), nil
	})
}
