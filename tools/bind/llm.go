package main

import (
	"encoding/json"

	"github.com/nonchan7720/spec-gate/pkg/services"
)

// llm-paths: {"ref": ref, "runs": 回数} → extract --emit が書くプロンプトのパス（ルート相対）の配列。
func init() {
	register("llm-paths", func(in json.RawMessage) (any, error) {
		var req struct {
			Ref  string `json:"ref"`
			Runs int    `json:"runs"`
		}
		if err := json.Unmarshal(in, &req); err != nil {
			return nil, err
		}
		return services.ExtractPromptPaths(req.Ref, req.Runs), nil
	})
}
