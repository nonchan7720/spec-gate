package main

import (
	"encoding/json"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// relink: {"missing": 見出し, "headings": [見出し...]} → 提案する見出し。無ければ null。
func init() {
	register("relink", func(in json.RawMessage) (any, error) {
		var req struct {
			Missing  string   `json:"missing"`
			Headings []string `json:"headings"`
		}
		if err := json.Unmarshal(in, &req); err != nil {
			return nil, err
		}
		h, ok := domain.SuggestHeading(req.Missing, req.Headings)
		if !ok {
			return nil, nil
		}
		return h, nil
	})
}
