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

func TestAutoApprovals_HumanReapprovalAndTimezones(t *testing.T) {
	ds := []ReviewDecision{
		{Time: "2026-11-04T01:00:00Z", ID: "A", Verdict: VerdictApprove, By: AutoApproverName},
		{Time: "2026-11-04T02:00:00Z", ID: "A", Verdict: VerdictApprove, By: "nonchan7720"},
		{Time: "2026-11-04T03:00:00Z", ID: "A", Verdict: VerdictRevoke, By: "nonchan7720"},
		{Time: "2026-11-04T04:00:00Z", ID: "B", Verdict: VerdictApprove, By: AutoApproverName},
		{Time: "2026-11-04T05:00:00Z", ID: "B", Verdict: VerdictRevoke, By: "nonchan7720"},
	}
	recs := AutoApprovals(ds)
	if recs[0].Revoked || !recs[1].Revoked {
		t.Fatalf("人間が承認し直した後の差し戻しは、前の自動承認に付けない: %+v", recs)
	}
	// 2026-11-04T13:00:00+09:00 は 04:00Z。05:00Z の差し戻しはその後
	if h := BreakerHistory(recs, "2026-11-04T13:00:00+09:00"); !reflect.DeepEqual(h, []bool{false, true}) {
		t.Fatalf("reset_at は時差つきでも時刻として比べる: %v", h)
	}
	if h := BreakerHistory(recs, "2026-11-04T15:00:00+09:00"); !reflect.DeepEqual(h, []bool{false, false}) {
		t.Fatalf("reset_at（06:00Z）より前の差し戻しは数えない: %v", h)
	}
}

func TestResolveAutoPolicy_Breaker(t *testing.T) {
	ip := func(v int) *int { return &v }
	sp := func(v string) *string { return &v }
	on := true
	for _, c := range []struct {
		window, max *int
		reset       *string
		bad         bool
	}{
		{nil, nil, nil, false},
		{ip(-1), nil, nil, true},
		{ip(0), nil, nil, true},
		{nil, ip(0), nil, true},
		{nil, nil, sp("昨日"), true},
		{nil, nil, sp("2026-11-04T13:00:00+09:00"), false},
	} {
		var f AutoPolicyFile
		f.AutoApprove = &struct {
			Enabled        *bool    `json:"enabled" yaml:"enabled"`
			LLMCommands    []string `json:"llm_commands" yaml:"llm_commands"`
			Runs           *int     `json:"runs" yaml:"runs"`
			MinAgree       *int     `json:"min_agree" yaml:"min_agree"`
			Kinds          []string `json:"kinds" yaml:"kinds"`
			RequireReverse *bool    `json:"require_reverse" yaml:"require_reverse"`
			Breaker        *struct {
				Window     *int    `json:"window" yaml:"window"`
				MaxRevoked *int    `json:"max_revoked" yaml:"max_revoked"`
				ResetAt    *string `json:"reset_at" yaml:"reset_at"`
			} `json:"breaker" yaml:"breaker"`
		}{Enabled: &on, LLMCommands: []string{"a"}}
		f.AutoApprove.Breaker = &struct {
			Window     *int    `json:"window" yaml:"window"`
			MaxRevoked *int    `json:"max_revoked" yaml:"max_revoked"`
			ResetAt    *string `json:"reset_at" yaml:"reset_at"`
		}{c.window, c.max, c.reset}
		_, err := ResolveAutoPolicy(&f)
		if (err != nil) != c.bad {
			t.Errorf("window=%v max=%v reset=%v: err=%v, want bad=%v", c.window, c.max, c.reset, err, c.bad)
		}
	}
	if AutoBreakerTripped(-1, 2, []bool{true, true}) != true {
		t.Error("window が不正でも落ちない（全件を数える）")
	}
}
