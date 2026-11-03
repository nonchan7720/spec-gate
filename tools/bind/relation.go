package main

import (
	"encoding/json"
	"errors"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// relation は docs/spec/harness.md「ランダム入力による検証」の binding。
// 入力 {"relation": 式, "x": 値, "values": {"f(x)": 値, ...}} の relation を、
// f(...) に values の値を当てはめて評価し、真偽を返す。
func relation(in json.RawMessage) (any, error) {
	var req struct {
		Relation string         `json:"relation"`
		X        any            `json:"x"`
		Values   map[string]any `json:"values"`
	}
	if err := json.Unmarshal(in, &req); err != nil {
		return nil, err
	}
	if req.Relation == "" {
		return nil, errors.New("relation が空")
	}
	if err := domain.CheckRelation(req.Relation); err != nil {
		return nil, err
	}
	return domain.EvalRelationValues(req.Relation, req.X, req.Values)
}

func init() { register("relation", relation) }
