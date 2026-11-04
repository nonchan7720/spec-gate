package services

import (
	"context"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

func TestCheck_ResultsCarryClaimKind(t *testing.T) {
	snaps := map[string]domain.Snapshot{"s.md#a": {Hash: "h1"}}
	claims := []domain.Claim{
		mdClaim("UNAPPROVED", "s.md#a", domain.KindUI, nil),
		mdClaim("MANUAL", "s.md#a", domain.KindManual, approved("h1")),
		mdClaim("PASS", "s.md#a", domain.KindExample, approved("h1")),
		mdClaim("STALE", "s.md#a", domain.KindExample, approved("old")),
	}
	s, _ := newCheck(claims, snaps, map[string]domain.Status{"PASS": domain.StatusPass})
	rep, err := s.Run(context.Background(), CheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]domain.ClaimKind{"UNAPPROVED": domain.KindUI, "MANUAL": domain.KindManual, "PASS": domain.KindExample, "STALE": domain.KindExample}
	for id, k := range want {
		if got := statusOf(t, rep, id); got.Kind != k {
			t.Errorf("%s の Kind = %q, want %q（役割別レポート・metrics が kind を使う）", id, got.Kind, k)
		}
	}
}

func TestComputeMetrics(t *testing.T) {
	rs := domain.Results{
		{ID: "A", Kind: domain.KindExample, Status: domain.StatusPass},
		{ID: "B", Kind: domain.KindExample, Status: domain.StatusFail},
		{ID: "C", Kind: domain.KindUI, Status: domain.StatusUnverified},
		{ID: "D", Kind: domain.KindManual, Status: domain.StatusUnverified},
	}
	m := ComputeMetrics(rs)
	if m.Total != 4 || m.Unverified != 2 || m.UnverifiedRate != 0.5 {
		t.Fatalf("総数・unverified 件数・率: %+v", m)
	}
	if len(m.ByKind) != 4 {
		t.Fatalf("4 種類すべてを 0 件でも並べる: %+v", m.ByKind)
	}
	want := []KindMetric{
		{Kind: domain.KindExample, Count: 2, Ratio: 0.5},
		{Kind: domain.KindStructure, Count: 0, Ratio: 0},
		{Kind: domain.KindUI, Count: 1, Ratio: 0.25},
		{Kind: domain.KindManual, Count: 1, Ratio: 0.25},
	}
	for i, w := range want {
		if m.ByKind[i] != w {
			t.Errorf("ByKind[%d] = %+v, want %+v", i, m.ByKind[i], w)
		}
	}
}

func TestComputeMetrics_Empty(t *testing.T) {
	m := ComputeMetrics(nil)
	if m.Total != 0 || m.UnverifiedRate != 0 {
		t.Fatalf("claim 0 件なら率は 0: %+v", m)
	}
	for _, k := range m.ByKind {
		if k.Count != 0 || k.Ratio != 0 {
			t.Fatalf("0 件のとき割合は 0（NaN にしない）: %+v", k)
		}
	}
}

func TestMetrics_WithApprovals(t *testing.T) {
	claims := []domain.Claim{
		mdClaim("A", "s.md#a", domain.KindExample, &domain.Approval{By: domain.AutoApproverName}),
		mdClaim("H", "s.md#a", domain.KindExample, approved("h")),
		mdClaim("P", "s.md#a", domain.KindExample, nil),
	}
	ds := []domain.ReviewDecision{
		{ID: "A", Verdict: domain.VerdictApprove, By: domain.AutoApproverName},
		{ID: "X", Verdict: domain.VerdictRevoke},
		{ID: "Y", Verdict: domain.VerdictRevoke},
		{ID: "Z", Verdict: domain.VerdictReject},
	}
	m := Metrics{}.WithApprovals(claims, ds)
	if m.AutoApproved != 1 || m.Revoked != 2 {
		t.Fatalf("自動承認の claim の数と差し戻しの数: %+v", m)
	}
}
