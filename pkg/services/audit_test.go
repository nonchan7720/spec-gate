package services

import (
	"encoding/json"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

func (m *memDecisions) All() ([]domain.ReviewDecision, error) { return m.got, nil }

type fakeRevoker struct {
	got    []string
	reason string
}

func (f *fakeRevoker) Revoke(c domain.Claim, reason string) (string, error) {
	f.got = append(f.got, c.ID)
	f.reason = reason
	return "claims/_pending/" + c.ID + ".yaml", nil
}

func TestAuditRevoke(t *testing.T) {
	claims := []domain.Claim{
		mdClaim("A", "s.md#a", domain.KindManual, approved("h")),
		mdClaim("P", "s.md#a", domain.KindManual, nil),
	}
	rv, log := &fakeRevoker{}, &memDecisions{}
	s := &AuditService{Claims: &fakeClaims{claims: claims}, Revoker: rv, Log: log, Clock: fixedClock("2026-11-04T09:00:00Z")}
	if _, err := s.Revoke("A", " ", "nonchan7720"); err == nil {
		t.Fatal("理由が無ければ取り消さない")
	}
	if _, err := s.Revoke("P", "x", "nonchan7720"); err == nil {
		t.Fatal("承認されていない claim は取り消せない")
	}
	path, err := s.Revoke("A", "例が\n違う", "nonchan7720")
	if err != nil || path != "claims/_pending/A.yaml" || len(rv.got) != 1 {
		t.Fatalf("承認を取り消す: %q %v %v", path, err, rv.got)
	}
	want := domain.ReviewDecision{Time: "2026-11-04T09:00:00Z", ID: "A", Verdict: domain.VerdictRevoke, Reason: "例が 違う", By: "nonchan7720"}
	if len(log.got) != 1 || log.got[0] != want {
		t.Fatalf("判定 revoke で記録する: %+v", log.got)
	}
}

func TestAuditList(t *testing.T) {
	ev, _ := json.Marshal(domain.Evidence{ID: "B", Ref: "s.md#b", Confidence: "3/3", Reverse: "match", DryRun: "pass", Verdict: "ok"})
	log := &memDecisions{got: []domain.ReviewDecision{
		{Time: "2026-11-04T01:00:00Z", ID: "A", Verdict: domain.VerdictApprove, By: domain.AutoApproverName},
		{Time: "2026-11-04T02:00:00Z", ID: "H", Verdict: domain.VerdictApprove, By: "nonchan7720"},
		{Time: "2026-11-04T03:00:00Z", ID: "B", Verdict: domain.VerdictApprove, By: domain.AutoApproverName, Evidence: ".spec-gate/evidence/B.json"},
		{Time: "2026-11-04T04:00:00Z", ID: "A", Verdict: domain.VerdictRevoke, By: "nonchan7720"},
	}}
	claims := []domain.Claim{mdClaim("B", "s.md#b", domain.KindExample, approved("h"))}
	s := &AuditService{Claims: &fakeClaims{claims: claims}, Decisions: log, Evidence: fakeEvidence{"B": ev}}
	items, err := s.List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != "B" || items[1].ID != "A" {
		t.Fatalf("自動承認だけを新しい順に: %+v", items)
	}
	if b := items[0]; b.Ref != "s.md#b" || b.Confidence != "3/3" || b.Reverse != "match" || b.DryRun != "pass" || b.Revoked || b.Title == "" {
		t.Errorf("証拠の中身と今の title を添える: %+v", b)
	}
	if !items[1].Revoked {
		t.Errorf("差し戻し済みを示す: %+v", items[1])
	}
	if items, _ := s.List("2026-11-04T02:00:00Z"); len(items) != 1 || items[0].ID != "B" {
		t.Errorf("since 以降に絞る: %+v", items)
	}
}
