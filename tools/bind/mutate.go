package main

import (
	"encoding/json"
	"fmt"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// mutate: 入力はコード 1 行、出力はその行に 1 か所だけ変異を入れた行の配列（docs/spec/harness.md「mutation score」）。
func init() {
	register("mutate", func(in json.RawMessage) (any, error) {
		var line string
		if err := json.Unmarshal(in, &line); err != nil {
			return nil, fmt.Errorf("入力はコード 1 行の文字列: %w", err)
		}
		return domain.MutateLine(line), nil
	})
}
