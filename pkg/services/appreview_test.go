package services

import (
	"context"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// fakeProbe は claim ID ごとに決めた試し実行の結果を返す。
type fakeProbe map[string]domain.DryRun

func (f fakeProbe) Probe(_ context.Context, c domain.Claim) domain.DryRun { return f[c.ID] }

func TestAppItems_OrderAndHash(t *testing.T) {
	a := pending(mdClaim("A", "s.md#a", domain.KindExample, nil), "1/3")
	b := pending(mdClaim("B", "s.md#a", domain.KindExample, nil), "3/3")
	b.DryRun = "pass"
	s, _, _ := newRemote([]domain.PendingClaim{a, b})
	items, err := s.AppItems(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != "B" || !items[0].Eligible || items[1].Eligible {
		t.Fatalf("承認しやすい順で、全員一致・pass がまとめ承認の対象: %+v", items)
	}
	if items[0].ContentSHA256 != contentSHA(items[0].Preview) || !strings.Contains(items[0].Preview, "試し実行: pass") {
		t.Fatalf("内容とその sha256: %+v", items[0])
	}
}

func TestAppDecide_RequiresShownContent(t *testing.T) {
	p := pending(mdClaim("A", "s.md#a", domain.KindExample, nil), "3/3")
	s, store, log := newRemote([]domain.PendingClaim{p})
	ctx := context.Background()
	err := s.AppDecide(ctx, AppDecision{Name: "A.yaml", Verdict: domain.VerdictApprove, ContentSHA256: "古い内容"}, "nonchan")
	if err != ErrContentChanged || len(store.approved) != 0 {
		t.Fatalf("見せた内容と違えば反映しない: %v", err)
	}
	sha := contentSHA(domain.RenderRemotePreview(p))
	if err := s.AppDecide(ctx, AppDecision{Name: "A.yaml", Verdict: domain.VerdictApprove, ContentSHA256: sha}, "nonchan"); err != nil {
		t.Fatal(err)
	}
	if len(store.approved) != 1 || len(log.got) != 1 || log.got[0].ContentSHA256 != sha || log.got[0].By != "nonchan" {
		t.Fatalf("承認して記録する: %+v %+v", store.approved, log.got)
	}
}

func TestAppBulkApprove_ReprobesAndChecksContent(t *testing.T) {
	a := pending(mdClaim("A", "s.md#a", domain.KindExample, nil), "3/3")
	b := pending(mdClaim("B", "s.md#a", domain.KindExample, nil), "3/3")
	c := pending(mdClaim("C", "s.md#a", domain.KindExample, nil), "3/3")
	s, store, log := newRemote([]domain.PendingClaim{a, b, c})
	s.Review.Approver.Probe = fakeProbe{"A": {Total: 1}, "B": {Total: 1, Mismatch: 1}, "C": {Total: 1}}
	sha := func(p domain.PendingClaim) string { return contentSHA(domain.RenderRemotePreview(p)) }
	out := s.AppBulkApprove(context.Background(), []AppDecision{
		{Name: "A.yaml", ContentSHA256: sha(a)},
		{Name: "B.yaml", ContentSHA256: sha(b)},
		{Name: "C.yaml", ContentSHA256: "差し替え"},
	}, "nonchan")
	errs := map[string]string{}
	for _, o := range out {
		errs[o.Name] = o.Error
	}
	if errs["A.yaml"] != "" || !strings.Contains(errs["B.yaml"], "pass でない") || errs["C.yaml"] != ErrContentChanged.Error() {
		t.Fatalf("pass のものだけ・見せた内容のままのものだけ承認する: %+v", out)
	}
	if len(store.approved) != 1 || store.approved[0].ID != "A" || len(log.got) != 1 {
		t.Fatalf("承認と記録は A だけ: %+v %+v", store.approved, log.got)
	}
}

func TestAppItems_LintRefusalIsShownAndNotEligible(t *testing.T) {
	c := mdClaim("A", "s.md#a", domain.KindExample, nil)
	c.Properties = []string{"output.contains('x')"} // 数値の出力に文字列のメソッド
	p := pending(c, "3/3")
	p.DryRun = "pass"
	s, _, _ := newRemote([]domain.PendingClaim{p})
	items, err := s.AppItems(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if items[0].Eligible || !strings.Contains(items[0].Error, "lint") {
		t.Fatalf("lint で拒否される候補は理由を出し、まとめ承認の対象にしない: %+v", items[0])
	}
}
