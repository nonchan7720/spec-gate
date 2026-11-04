package main

import (
	"encoding/json"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// docs/spec/auto-approval.md（承認の自動化）の binding。
func init() {
	// auto-policy: policy.yaml を読んだ値（無ければ null）→ 既定値を埋めた方針、無効なら {"enabled": false}、誤りなら {"error": 項目名}
	register("auto-policy", func(in json.RawMessage) (any, error) {
		var f *domain.AutoPolicyFile
		if err := json.Unmarshal(in, &f); err != nil {
			return nil, err
		}
		p, err := domain.ResolveAutoPolicy(f)
		if perr, ok := err.(*domain.AutoPolicyError); ok {
			return map[string]string{"error": perr.Field}, nil
		}
		if err != nil {
			return nil, err
		}
		if !p.Enabled {
			return map[string]bool{"enabled": false}, nil
		}
		return p, nil
	})

	// auto-run-commands: {"commands", "runs"} → run ごとに使うコマンドの配列
	register("auto-run-commands", func(in json.RawMessage) (any, error) {
		var req struct {
			Commands []string `json:"commands"`
			Runs     int      `json:"runs"`
		}
		if err := json.Unmarshal(in, &req); err != nil {
			return nil, err
		}
		return domain.AutoRunCommands(req.Commands, req.Runs), nil
	})

	// auto-eligible: {"policy", "paused", "candidate"} → 判定（ok / disabled / paused / kind / agree / ambiguous / dry-run / reverse）
	register("auto-eligible", func(in json.RawMessage) (any, error) {
		var req struct {
			Policy struct {
				Enabled        bool     `json:"enabled"`
				Kinds          []string `json:"kinds"`
				MinAgree       int      `json:"min_agree"`
				RequireReverse bool     `json:"require_reverse"`
			} `json:"policy"`
			Paused    bool                 `json:"paused"`
			Candidate domain.AutoCandidate `json:"candidate"`
		}
		if err := json.Unmarshal(in, &req); err != nil {
			return nil, err
		}
		p := domain.AutoPolicy{Enabled: req.Policy.Enabled, Kinds: req.Policy.Kinds, MinAgree: req.Policy.MinAgree, RequireReverse: req.Policy.RequireReverse}
		return domain.AutoEligibility(p, req.Paused, req.Candidate), nil
	})

	// auto-evidence: {"by", "evidence"} → {"approved", "detail"}
	register("auto-evidence", func(in json.RawMessage) (any, error) {
		var req struct {
			By       string                    `json:"by"`
			Evidence *domain.AutoEvidenceState `json:"evidence"`
		}
		if err := json.Unmarshal(in, &req); err != nil {
			return nil, err
		}
		ok, detail := domain.AutoEvidenceCheck(req.By, req.Evidence)
		return map[string]any{"approved": ok, "detail": detail}, nil
	})

	// auto-breaker: {"window", "max_revoked", "history": [{"revoked"}]} → 止めるか
	register("auto-breaker", func(in json.RawMessage) (any, error) {
		var req struct {
			Window     int `json:"window"`
			MaxRevoked int `json:"max_revoked"`
			History    []struct {
				Revoked bool `json:"revoked"`
			} `json:"history"`
		}
		if err := json.Unmarshal(in, &req); err != nil {
			return nil, err
		}
		revoked := make([]bool, len(req.History))
		for i, h := range req.History {
			revoked[i] = h.Revoked
		}
		return domain.AutoBreakerTripped(req.Window, req.MaxRevoked, revoked), nil
	})

	// guard: {"tool", "command" か "path", "agent_type"} → "allow" / "deny"
	register("guard", func(in json.RawMessage) (any, error) {
		var req domain.GuardInput
		if err := json.Unmarshal(in, &req); err != nil {
			return nil, err
		}
		return domain.Guard(req, "").Verdict, nil
	})
}
