package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

func newCheck(claims []domain.Claim, snaps map[string]domain.Snapshot, results map[string]domain.Status) (*CheckService, *fakeChecker) {
	chk := &fakeChecker{kind: domain.KindExample, results: results}
	return &CheckService{
		Claims:   &fakeClaims{claims: claims},
		Plans:    &fakePlans{plans: map[string]domain.Plan{}, locks: map[string]*domain.PlanLock{}},
		Adapters: map[domain.SourceType]domain.SourceAdapter{domain.SourceMarkdown: fakeAdapter{typ: domain.SourceMarkdown, snaps: snaps}},
		Checkers: map[domain.ClaimKind]domain.Checker{domain.KindExample: chk},
		Runner:   &fakeRunner{},
	}, chk
}

func statusOf(t *testing.T, rep CheckReport, id string) domain.Result {
	t.Helper()
	for _, r := range rep.Claims {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("%s の結果が無い: %+v", id, rep.Claims)
	return domain.Result{}
}

func TestCheck_OrderOfStages(t *testing.T) {
	snaps := map[string]domain.Snapshot{
		"s.md#a": {Hash: "h1", Location: "s.md:3"},
		"s.md#b": {Hash: "h2-changed", Excerpt: "新しい仕様"},
		"s.md#c": {Skipped: "見出しが無い"},
	}
	claims := []domain.Claim{
		mdClaim("UNAPPROVED", "s.md#a", domain.KindExample, nil),
		mdClaim("STALE", "s.md#b", domain.KindExample, approved("h2")),
		mdClaim("UNREADABLE", "s.md#c", domain.KindExample, approved("h3")),
		mdClaim("MANUAL", "s.md#a", domain.KindManual, approved("h1")),
		mdClaim("FAIL", "s.md#a", domain.KindExample, approved("h1")),
		mdClaim("PASS", "s.md#a", domain.KindExample, approved("h1")),
	}
	s, chk := newCheck(claims, snaps, map[string]domain.Status{"FAIL": domain.StatusFail, "PASS": domain.StatusPass, "STALE": domain.StatusPass, "UNAPPROVED": domain.StatusPass})
	rep, err := s.Run(context.Background(), CheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]domain.Status{
		"UNAPPROVED": domain.StatusUnapproved, // 承認が最優先。checker を通っても pass にしない
		"STALE":      domain.StatusStale,      // 仕様が承認後に変わったら、コードが合っていても stale
		"UNREADABLE": domain.StatusFail,       // drift: required で仕様が読めなければ fail
		"MANUAL":     domain.StatusUnverified,
		"FAIL":       domain.StatusFail,
		"PASS":       domain.StatusPass,
	}
	for id, st := range want {
		if got := statusOf(t, rep, id); got.Status != st {
			t.Errorf("%s = %s, want %s (%s)", id, got.Status, st, got.Detail)
		}
	}
	if r := statusOf(t, rep, "STALE"); !strings.Contains(r.Detail, "新しい仕様") {
		t.Errorf("stale には現在の仕様を添える: %q", r.Detail)
	}
	if len(chk.opts) != 2 {
		t.Errorf("checker は承認済み・ドリフト無しの claim だけに呼ぶ: %d 回", len(chk.opts))
	}
	if !rep.HasBlocking() {
		t.Error("fail / stale / unapproved があれば blocking")
	}
}

func TestCheck_DriftOptionalPassesWithNote(t *testing.T) {
	c := mdClaim("OPT", "s.md#c", domain.KindExample, approved("x"))
	c.Source.Drift = "optional"
	s, _ := newCheck([]domain.Claim{c}, map[string]domain.Snapshot{"s.md#c": {Skipped: "FIGMA_TOKEN が無い"}}, map[string]domain.Status{"OPT": domain.StatusPass})
	rep, err := s.Run(context.Background(), CheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	r := statusOf(t, rep, "OPT")
	if r.Status != domain.StatusPass || !strings.Contains(r.Detail, "ドリフト検知は無効") {
		t.Fatalf("drift: optional は判定を続け、ドリフト検知が無効だったことを残す: %+v", r)
	}
}

func TestCheck_UnknownKindAndAdapterFail(t *testing.T) {
	ui := mdClaim("UI", "s.md#a", domain.KindUI, approved("h1"))
	fig := mdClaim("FIG", "figma://k/n", domain.KindExample, approved("h"))
	fig.Source.Type = domain.SourceFigma
	s, _ := newCheck([]domain.Claim{ui, fig}, map[string]domain.Snapshot{"s.md#a": {Hash: "h1"}}, nil)
	rep, err := s.Run(context.Background(), CheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if r := statusOf(t, rep, "UI"); r.Status != domain.StatusFail || !strings.Contains(r.Detail, "checker が未登録") {
		t.Errorf("checker の無い kind は pass にしない: %+v", r)
	}
	if r := statusOf(t, rep, "FIG"); r.Status != domain.StatusFail || !strings.Contains(r.Detail, "adapter が未登録") {
		t.Errorf("adapter の無い source は pass にしない: %+v", r)
	}
}

func TestCheck_OnlyFiltersAndSkipsPlans(t *testing.T) {
	snaps := map[string]domain.Snapshot{"s.md#a": {Hash: "h1"}}
	claims := []domain.Claim{mdClaim("A", "s.md#a", domain.KindExample, approved("h1")), mdClaim("B", "s.md#a", domain.KindExample, approved("h1"))}
	s, _ := newCheck(claims, snaps, map[string]domain.Status{"A": domain.StatusPass, "B": domain.StatusPass})
	s.Plans = &fakePlans{plans: map[string]domain.Plan{"plans/p.md": {Path: "plans/p.md", Status: "active"}}, locks: map[string]*domain.PlanLock{}}
	rep, err := s.Run(context.Background(), CheckRequest{Only: []string{" B "}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Claims) != 1 || rep.Claims[0].ID != "B" || rep.Skipped != 1 || len(rep.Plans) != 0 {
		t.Fatalf("--only は指定 ID だけ、プランは見ない: %+v", rep)
	}
}

func TestCheck_ChangedFiltersByTouchedFiles(t *testing.T) {
	snaps := map[string]domain.Snapshot{"docs/a.md#x": {Hash: "h"}, "docs/b.md#x": {Hash: "h"}}
	a := mdClaim("A", "docs/a.md#x", domain.KindExample, approved("h"))
	a.Binding = "npx tsx tools/call.ts src/a.ts f"
	b := mdClaim("B", "docs/b.md#x", domain.KindExample, approved("h"))
	b.Binding = "npx tsx tools/call.ts src/b.ts f"
	results := map[string]domain.Status{"A": domain.StatusPass, "B": domain.StatusPass}

	cases := []struct {
		name    string
		changed []string
		want    []string
	}{
		{"binding のファイル", []string{"src/a.ts"}, []string{"A"}},
		{"仕様ファイル", []string{"docs/b.md"}, []string{"B"}},
		{"無関係", []string{"README.md"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := newCheck([]domain.Claim{a, b}, snaps, results)
			s.Changes = fakeChanges{files: tc.changed}
			rep, err := s.Run(context.Background(), CheckRequest{Changed: true})
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, r := range rep.Claims {
				got = append(got, r.ID)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("git でなければ全件", func(t *testing.T) {
		s, _ := newCheck([]domain.Claim{a, b}, snaps, results)
		s.Changes = fakeChanges{files: nil}
		rep, _ := s.Run(context.Background(), CheckRequest{Changed: true})
		if len(rep.Claims) != 2 {
			t.Fatalf("変更一覧が取れなければ見逃さないよう全件: %+v", rep.Claims)
		}
	})
	t.Run("変更一覧の取得エラーは返す", func(t *testing.T) {
		s, _ := newCheck([]domain.Claim{a, b}, snaps, results)
		s.Changes = fakeChanges{err: errors.New("git broke")}
		if _, err := s.Run(context.Background(), CheckRequest{Changed: true}); err == nil {
			t.Fatal("取得エラーを握りつぶさない")
		}
	})
}

func TestCheck_Plans(t *testing.T) {
	frozen := planWith("plans/ok.md", "true", "false && boom")
	frozen.Status = "active"
	tampered := planWith("plans/tampered.md", "true")
	tampered.Status = "active"
	unfrozen := planWith("plans/new.md", "true")
	unfrozen.Status = "active"
	missing := planWith("plans/missing.md", "true", "")
	missing.Status = "active"
	draft := planWith("plans/draft.md", "false")

	plans := &fakePlans{
		plans: map[string]domain.Plan{"plans/ok.md": frozen, "plans/tampered.md": tampered, "plans/new.md": unfrozen, "plans/missing.md": missing, "plans/draft.md": draft},
		locks: map[string]*domain.PlanLock{
			"plans/ok.md":       {Hash: frozen.VerifyHash()},
			"plans/tampered.md": {Hash: planWith("plans/tampered.md", "go test ./...").VerifyHash()},
		},
	}
	runner := &fakeRunner{}
	s := &CheckService{Claims: &fakeClaims{}, Plans: plans, Runner: runner}
	rep, err := s.Run(context.Background(), CheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]PlanReport{}
	for _, p := range rep.Plans {
		byPath[p.Path] = p
	}
	if _, ok := byPath["plans/draft.md"]; ok || len(rep.Plans) != 4 {
		t.Fatalf("active なプランだけ判定する: %+v", rep.Plans)
	}
	if it := byPath["plans/ok.md"].Items; len(it) != 2 || it[0].Status != domain.StatusPass || it[1].Status != domain.StatusFail || !strings.Contains(it[1].Detail, "boom") {
		t.Errorf("凍結済みなら verify を実行し、失敗は出力つき fail: %+v", it)
	}
	if it := byPath["plans/tampered.md"].Items; len(it) != 1 || it[0].Status != domain.StatusStale {
		t.Errorf("verify が凍結後に変わったら stale（verify は実行しない）: %+v", it)
	}
	if it := byPath["plans/new.md"].Items; len(it) != 1 || it[0].Status != domain.StatusUnapproved {
		t.Errorf("未凍結は unapproved: %+v", it)
	}
	if it := byPath["plans/missing.md"].Items; len(it) != 1 || it[0].Status != domain.StatusFail || !strings.Contains(it[0].Title, "verify のない項目") {
		t.Errorf("verify の無い項目があれば fail: %+v", it)
	}
	for _, c := range runner.calls {
		if c == "go test ./..." || strings.HasPrefix(c, "false") && c != "false && boom" {
			t.Errorf("凍結されていない / 改ざんされた verify を実行した: %q", c)
		}
	}
}
