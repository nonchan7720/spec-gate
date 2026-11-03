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

	// 入力は {"role", "results": [{"id", "kind", "status"}]}、出力は出す claim の id の配列
	register("role-filter", func(in json.RawMessage) (any, error) {
		var req struct {
			Role    string `json:"role"`
			Results []struct {
				ID     string           `json:"id"`
				Kind   domain.ClaimKind `json:"kind"`
				Status domain.Status    `json:"status"`
			} `json:"results"`
		}
		if err := json.Unmarshal(in, &req); err != nil {
			return nil, fmt.Errorf("入力は {\"role\", \"results\"}: %w", err)
		}
		role, err := domain.ParseRole(req.Role)
		if err != nil {
			return nil, err
		}
		rs := make(domain.Results, 0, len(req.Results))
		for _, r := range req.Results {
			rs = append(rs, domain.Result{ID: r.ID, Kind: r.Kind, Status: r.Status})
		}
		ids := []string{}
		for _, r := range domain.FilterByRole(role, rs) {
			ids = append(ids, r.ID)
		}
		return ids, nil
	})
}
