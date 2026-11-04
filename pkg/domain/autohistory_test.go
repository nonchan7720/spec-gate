package domain

import (
	"reflect"
	"testing"
)

func TestAutoApprovals(t *testing.T) {
	ds := []ReviewDecision{
		{Time: "2026-11-04T01:00:00Z", ID: "A", Verdict: VerdictApprove, By: AutoApproverName, Evidence: ".spec-gate/evidence/A.json"},
		{Time: "2026-11-04T02:00:00Z", ID: "H", Verdict: VerdictApprove, By: "nonchan7720"},
		{Time: "2026-11-04T03:00:00Z", ID: "B", Verdict: VerdictApprove, By: AutoApproverName},
		{Time: "2026-11-04T04:00:00Z", ID: "A", Verdict: VerdictRevoke, By: "nonchan7720", Reason: "誤り"},
		{Time: "2026-11-04T05:00:00Z", ID: "H", Verdict: VerdictRevoke, By: "nonchan7720"},
		{Time: "2026-11-04T06:00:00Z", ID: "A", Verdict: VerdictApprove, By: AutoApproverName},
	}
	got := AutoApprovals(ds)
	want := []AutoApprovalRecord{
		{ID: "A", Time: "2026-11-04T01:00:00Z", Evidence: ".spec-gate/evidence/A.json", Revoked: true, RevokedAt: "2026-11-04T04:00:00Z"},
		{ID: "B", Time: "2026-11-04T03:00:00Z"},
		{ID: "A", Time: "2026-11-04T06:00:00Z"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("人間の承認は数えず、差し戻しはその前の自動承認に付ける:\n got %+v\nwant %+v", got, want)
	}
	if h := BreakerHistory(got, ""); !reflect.DeepEqual(h, []bool{true, false, false}) {
		t.Fatalf("reset_at が空なら全部数える: %v", h)
	}
	if h := BreakerHistory(got, "2026-11-04T05:00:00Z"); !reflect.DeepEqual(h, []bool{false, false, false}) {
		t.Fatalf("reset_at より前の差し戻しは数えない: %v", h)
	}
}
