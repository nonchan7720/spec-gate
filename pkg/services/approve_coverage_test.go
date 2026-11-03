package services

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

func newApprove(claims []domain.Claim, snaps map[string]domain.Snapshot) (*ApproveService, *fakeClaims) {
	repo := &fakeClaims{claims: claims}
	return &ApproveService{
		Claims:   repo,
		Adapters: map[domain.SourceType]domain.SourceAdapter{domain.SourceMarkdown: fakeAdapter{typ: domain.SourceMarkdown, snaps: snaps}},
		Clock:    fixedClock("2026-11-04T00:00:00Z"),
	}, repo
}

func TestApprove_RecordsHashAndSaves(t *testing.T) {
	s, repo := newApprove([]domain.Claim{mdClaim("A", "s.md#a", domain.KindExample, nil)}, map[string]domain.Snapshot{"s.md#a": {Hash: "h1", Excerpt: "仕様の本文"}})
	res, err := s.Approve(context.Background(), "A", "alice")
	if err != nil {
		t.Fatal(err)
	}
	want := domain.Approval{By: "alice", At: "2026-11-04T00:00:00Z", SourceHash: "h1"}
	if len(repo.saved) != 1 || *repo.saved[0].Approved != want || res.Excerpt != "仕様の本文" {
		t.Fatalf("承認者・日時・仕様のハッシュを記録して保存し、抜粋を返す: %+v %+v", repo.saved, res)
	}
}

func TestApprove_Rejections(t *testing.T) {
	conflictA := mdClaim("A", "s.md#a", domain.KindExample, nil)
	conflictA.Examples = []domain.Example{{In: 10, Out: 10}}
	conflictB := mdClaim("B", "s.md#a", domain.KindExample, nil)
	conflictB.Examples = []domain.Example{{In: 10, Out: 11}}
	unreadable := mdClaim("U", "s.md#missing", domain.KindExample, nil)

	cases := []struct {
		name string
		id   string
		want string
	}{
		{"存在しない claim", "NOPE", "がない"},
		{"lint に失敗（他の claim と矛盾）", "B", "lint に失敗"},
		{"仕様が読めない（drift: required）", "U", "仕様を読めない"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, repo := newApprove([]domain.Claim{conflictA, conflictB, unreadable}, map[string]domain.Snapshot{"s.md#a": {Hash: "h1"}, "s.md#missing": {Skipped: "見出しが無い"}})
			_, err := s.Approve(context.Background(), tc.id, "alice")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			if len(repo.saved) != 0 {
				t.Fatalf("拒否したら保存しない: %+v", repo.saved)
			}
		})
	}
}

func TestApprove_LintIssuesOfOtherClaimsDoNotBlock(t *testing.T) {
	a := mdClaim("A", "s.md#a", domain.KindExample, nil)
	a.Examples = []domain.Example{{In: 10, Out: 10}}
	b := mdClaim("B", "s.md#a", domain.KindExample, nil)
	b.Examples = []domain.Example{{In: 10, Out: 11}} // B 側に矛盾が出る
	s, repo := newApprove([]domain.Claim{a, b}, map[string]domain.Snapshot{"s.md#a": {Hash: "h1"}})
	if _, err := s.Approve(context.Background(), "A", "alice"); err != nil || len(repo.saved) != 1 {
		t.Fatalf("自分に関係しない lint 指摘では承認を止めない: %v", err)
	}
}

func TestCoverage_Report(t *testing.T) {
	claims := []domain.Claim{
		mdClaim("R", "spec.md#端数処理", domain.KindExample, nil),
		{ID: "ARCH", Kind: domain.KindStructure, Title: "t", Source: domain.Source{Type: domain.SourceMermaid, Ref: "arch.mmd"}, Layers: map[string]string{"app": "src/app", "domain": "src/domain"}},
	}
	md := fakeAdapter{typ: domain.SourceMarkdown, units: map[string][]string{"spec.md": {"spec.md#背景", "spec.md#端数処理", "spec.md#月末処理"}}}
	mm := fakeAdapter{typ: domain.SourceMermaid, units: map[string][]string{"arch.mmd": {"arch.mmd#app -> domain", "arch.mmd#app -> infra"}}}
	s := &CoverageService{
		Claims:   &fakeClaims{claims: claims},
		Sources:  fakeSources{{Type: domain.SourceMarkdown, Path: "spec.md", NonNormative: []string{"背景"}}, {Type: domain.SourceMermaid, Path: "arch.mmd"}, {Type: domain.SourceImage, Path: "x.png"}},
		Adapters: map[domain.SourceType]domain.SourceAdapter{domain.SourceMarkdown: md, domain.SourceMermaid: mm},
	}
	rep, err := s.Report(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Total != 2 || rep.ByKind[domain.KindExample] != 1 || rep.ByKind[domain.KindStructure] != 1 {
		t.Errorf("kind 別件数: %+v", rep.ByKind)
	}
	if len(rep.Entries) != 3 {
		t.Fatalf("entries: %+v", rep.Entries)
	}
	m := rep.Entries[0]
	if !reflect.DeepEqual(m.Covered, []string{"spec.md#端数処理"}) || !reflect.DeepEqual(m.Uncovered, []string{"spec.md#月末処理"}) || !reflect.DeepEqual(m.NonNormative, []string{"spec.md#背景"}) {
		t.Errorf("markdown: %+v", m)
	}
	g := rep.Entries[1]
	if !reflect.DeepEqual(g.Covered, []string{"arch.mmd#app -> domain"}) || !reflect.DeepEqual(g.Uncovered, []string{"arch.mmd#app -> infra"}) {
		t.Errorf("mermaid は両端が structure claim の層にあるエッジだけカバー: %+v", g)
	}
	if !strings.Contains(rep.Entries[2].Error, "adapter 未登録") {
		t.Errorf("adapter の無い source はエラーとして残す: %+v", rep.Entries[2])
	}
}
