package main

import (
	"encoding/json"
	"fmt"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// harness.md「リモート承認」の binding。
func init() {
	// 入力は {"hook": PostToolUse hook の stdin, "previews": {候補 ID: spec-gate が今作る preview}}、
	// 出力は反映する判定の配列 [{"id", "verdict", "reason"}]
	register("remote-decisions", func(in json.RawMessage) (any, error) {
		var req struct {
			Hook     domain.RemoteHookInput `json:"hook"`
			Previews map[string]string      `json:"previews"`
		}
		if err := json.Unmarshal(in, &req); err != nil {
			return nil, fmt.Errorf("入力は {\"hook\", \"previews\"}: %w", err)
		}
		current := map[string]domain.AskQuestion{}
		for id, p := range req.Previews {
			current[id] = domain.NewRemoteQuestion(id, p)
		}
		decisions, _ := domain.DecideRemote(req.Hook, current)
		return decisions, nil
	})
}
