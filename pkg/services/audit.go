package services

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// AuditService は自動承認の見張り（audit）と差し戻し（revoke）。docs/spec/auto-approval.md「見張りと差し戻し」。
type AuditService struct {
	Claims    domain.ClaimRepository
	Revoker   domain.RevokeRepository
	Log       domain.ReviewDecisionLog
	Decisions domain.ReviewDecisionReader
	Evidence  domain.EvidenceStore
	Clock     domain.Clock
}

// Revoke は承認済みの claim id の承認を取り消して _pending に戻し、判定 revoke を記録する。理由は必須。
// 自動承認でない claim も取り消せる。戻したファイルの相対パスを返す。
func (s *AuditService) Revoke(id, reason, by string) (string, error) {
	if strings.TrimSpace(reason) == "" {
		return "", fmt.Errorf("--reason（取り消す理由）は必須")
	}
	claims, err := s.Claims.All()
	if err != nil {
		return "", err
	}
	i := slices.IndexFunc(claims, func(c domain.Claim) bool { return c.ID == id && c.Approved != nil })
	if i < 0 {
		return "", fmt.Errorf("承認済みの claim %s がない", id)
	}
	path, err := s.Revoker.Revoke(claims[i], reason)
	if err != nil {
		return "", err
	}
	entry := domain.ReviewDecision{Time: s.Clock.Now(), ID: id, Verdict: domain.VerdictRevoke, Reason: strings.Join(strings.Fields(reason), " "), By: by}
	if err := s.Log.Append(entry); err != nil {
		return path, fmt.Errorf("取り消したが記録に失敗: %w", err)
	}
	return path, nil
}

// AuditItem は自動承認 1 件の一覧の行。
type AuditItem struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Ref        string `json:"ref"`
	Confidence string `json:"confidence"`
	Reverse    string `json:"reverse"`
	DryRun     string `json:"dry_run"`
	Time       string `json:"time"`
	Evidence   string `json:"evidence"`
	Revoked    bool   `json:"revoked"`
}

// List は since（RFC 3339。空なら全部）以降の自動承認を新しい順に返す。
func (s *AuditService) List(since string) ([]AuditItem, error) {
	ds, err := s.Decisions.All()
	if err != nil {
		return nil, err
	}
	claims, err := s.Claims.All()
	if err != nil {
		return nil, err
	}
	titles := map[string]string{}
	for _, c := range claims {
		titles[c.ID] = c.Title
	}
	recs := domain.AutoApprovals(ds)
	var out []AuditItem
	for _, r := range slices.Backward(recs) {
		if since != "" && r.Time < since {
			continue
		}
		it := AuditItem{ID: r.ID, Title: titles[r.ID], Time: r.Time, Evidence: r.Evidence, Revoked: r.Revoked}
		if raw, ok, err := s.Evidence.Load(r.ID); err == nil && ok {
			var e domain.Evidence
			if json.Unmarshal(raw, &e) == nil {
				it.Ref, it.Confidence, it.Reverse, it.DryRun = e.Ref, e.Confidence, e.Reverse, e.DryRun
			}
		}
		out = append(out, it)
	}
	return out, nil
}
