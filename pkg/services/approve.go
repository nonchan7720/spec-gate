package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// ApproveService は人間の承認を記録する。承認前に lint を通し、矛盾した claim は承認させない。
type ApproveService struct {
	Claims   domain.ClaimRepository
	Adapters map[domain.SourceType]domain.SourceAdapter
	Clock    domain.Clock
}

type ApproveResult struct {
	Claim   domain.Claim
	Excerpt string
}

func (s *ApproveService) Approve(ctx context.Context, id, by string) (ApproveResult, error) {
	claims, err := s.Claims.All()
	if err != nil {
		return ApproveResult{}, err
	}
	var target *domain.Claim
	for i := range claims {
		if claims[i].ID == id {
			target = &claims[i]
		}
	}
	if target == nil {
		return ApproveResult{}, fmt.Errorf("claim %s がない", id)
	}

	if issues := domain.Lint(claims); len(issues) > 0 {
		var mine []string
		for _, is := range issues {
			if is.ClaimID == id {
				mine = append(mine, is.Message)
			}
		}
		if len(mine) > 0 {
			return ApproveResult{}, fmt.Errorf("lint に失敗。承認できない:\n  %s", strings.Join(mine, "\n  "))
		}
	}

	ad, ok := s.Adapters[target.Source.Type]
	if !ok {
		return ApproveResult{}, fmt.Errorf("source type %s の adapter が未登録", target.Source.Type)
	}
	snap, err := ad.Snapshot(ctx, target.Source.Ref)
	if err != nil {
		return ApproveResult{}, err
	}
	if snap.Skipped != "" && target.Source.DriftRequired() {
		return ApproveResult{}, fmt.Errorf("仕様を読めないので承認できない: %s（drift: optional にすれば承認可）", snap.Skipped)
	}

	target.Approved = &domain.Approval{By: by, At: s.Clock.Now(), SourceHash: snap.Hash}
	if err := s.Claims.Save(*target); err != nil {
		return ApproveResult{}, err
	}
	return ApproveResult{Claim: *target, Excerpt: snap.Excerpt}, nil
}
