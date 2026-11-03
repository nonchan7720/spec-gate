package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// fakeReverser は claim の title から再生成文を作り、比較結果は再生成文ごとに決める。
type fakeReverser struct {
	restateErr map[string]error             // claim ID → Restate のエラー
	compare    map[string]domain.Comparison // 再生成文 → 比較結果
	compareErr error
	restated   []string // Restate に渡された claim ID
	originals  []string // Compare に渡された元の仕様
}

func (f *fakeReverser) Restate(_ context.Context, c domain.Claim) (string, error) {
	f.restated = append(f.restated, c.ID)
	if err := f.restateErr[c.ID]; err != nil {
		return "", err
	}
	return "再生成: " + c.Title, nil
}

func (f *fakeReverser) Compare(_ context.Context, restated, original string) (domain.Comparison, error) {
	f.originals = append(f.originals, original)
	if f.compareErr != nil {
		return domain.Comparison{}, f.compareErr
	}
	return f.compare[restated], nil
}

func newReverseFixture(rv *fakeReverser) *ReverseService {
	claims := []domain.Claim{
		mdClaim("A-1", "spec.md#a", domain.KindExample, approved("h")),
		mdClaim("A-2", "spec.md#b", domain.KindExample, approved("h")),
		mdClaim("A-3", "spec.md#gone", domain.KindExample, approved("h")),
		{ID: "UI-1", Kind: domain.KindUI, Title: "UI-1", Source: domain.Source{Type: domain.SourceFigma, Ref: "figma://k/n"}, Strings: []string{"一覧"}},
	}
	md := fakeAdapter{typ: domain.SourceMarkdown, snaps: map[string]domain.Snapshot{
		"spec.md#a":    {Hash: "h", Excerpt: "仕様 a", Location: "spec.md:1"},
		"spec.md#b":    {Hash: "h", Excerpt: "仕様 b", Location: "spec.md:5"},
		"spec.md#gone": {Skipped: "見出しが無い"},
	}}
	return &ReverseService{
		Claims:   &fakeClaims{claims: claims},
		Adapters: map[domain.SourceType]domain.SourceAdapter{domain.SourceMarkdown: md},
		Reverser: rv,
	}
}

func byID(rep ReverseReport) map[string]domain.ReverseResult {
	m := map[string]domain.ReverseResult{}
	for _, r := range rep.Results {
		m[r.ID] = r
	}
	return m
}

func TestReverse_MatchMismatchSkipped(t *testing.T) {
	rv := &fakeReverser{compare: map[string]domain.Comparison{
		"再生成: A-1": {Match: true},
		"再生成: A-2": {Match: false, Differences: []string{"仕様にある負数の扱いが claim に無い"}},
	}}
	rep, err := newReverseFixture(rv).Reverse(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Results) != 4 {
		t.Fatalf("全 claim を対象にする: %+v", rep.Results)
	}
	got := byID(rep)
	if r := got["A-1"]; r.Status != domain.ReverseMatch || r.Restated != "再生成: A-1" || r.Spec != "spec.md:1" {
		t.Fatalf("A-1: %+v", r)
	}
	if r := got["A-2"]; r.Status != domain.ReverseMismatch || len(r.Differences) != 1 || r.Restated == "" {
		t.Fatalf("A-2: %+v", r)
	}
	if r := got["A-3"]; r.Status != domain.ReverseSkipped || !strings.Contains(r.Detail, "見出しが無い") {
		t.Fatalf("仕様が読めなければ skipped: %+v", r)
	}
	if r := got["UI-1"]; r.Status != domain.ReverseSkipped || !strings.Contains(r.Detail, "figma") {
		t.Fatalf("テキストを持たない source は skipped: %+v", r)
	}
	// 仕様が読めない claim では LLM を呼ばない
	if strings.Join(rv.restated, ",") != "A-1,A-2" {
		t.Fatalf("Restate の対象: %v", rv.restated)
	}
	// Compare には元の仕様の抜粋を渡す
	if strings.Join(rv.originals, ",") != "仕様 a,仕様 b" {
		t.Fatalf("Compare に渡した仕様: %v", rv.originals)
	}
	if rep.Count(domain.ReverseMatch) != 1 || rep.Count(domain.ReverseMismatch) != 1 || rep.Count(domain.ReverseSkipped) != 2 {
		t.Fatalf("集計: %+v", rep)
	}
}

func TestReverse_MismatchWithoutDifferencesStaysMismatch(t *testing.T) {
	rv := &fakeReverser{compare: map[string]domain.Comparison{"再生成: A-1": {Match: false}}}
	rep, err := newReverseFixture(rv).Reverse(context.Background(), []string{"A-1"})
	if err != nil {
		t.Fatal(err)
	}
	if r := rep.Results[0]; r.Status != domain.ReverseMismatch {
		t.Fatalf("differences が空でも match=false は mismatch: %+v", r)
	}
}

func TestReverse_SelectedIDs(t *testing.T) {
	rv := &fakeReverser{compare: map[string]domain.Comparison{"再生成: A-2": {Match: true}}}
	rep, err := newReverseFixture(rv).Reverse(context.Background(), []string{"A-2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Results) != 1 || rep.Results[0].ID != "A-2" {
		t.Fatalf("指定した claim だけ: %+v", rep.Results)
	}
}

func TestReverse_UnknownIDIsError(t *testing.T) {
	// _pending の claim は ClaimRepository.All が返さないので、ID 指定でもここに来る
	_, err := newReverseFixture(&fakeReverser{}).Reverse(context.Background(), []string{"NOPE-1"})
	if err == nil || !strings.Contains(err.Error(), "NOPE-1") {
		t.Fatalf("存在しない claim は error: %v", err)
	}
}

func TestReverse_LLMErrorsAreNeverMatch(t *testing.T) {
	rv := &fakeReverser{
		restateErr: map[string]error{"A-1": errors.New("command failed")},
		compareErr: errors.New("応答に JSON が無い"),
	}
	rep, err := newReverseFixture(rv).Reverse(context.Background(), []string{"A-1", "A-2"})
	if err != nil {
		t.Fatal(err)
	}
	got := byID(rep)
	if r := got["A-1"]; r.Status != domain.ReverseError || !strings.Contains(r.Detail, "command failed") {
		t.Fatalf("Restate の失敗は error: %+v", r)
	}
	if r := got["A-2"]; r.Status != domain.ReverseError || !strings.Contains(r.Detail, "JSON") || r.Restated == "" {
		t.Fatalf("Compare の失敗は error（再生成文は残す）: %+v", r)
	}
}

func TestReverse_SnapshotErrorIsError(t *testing.T) {
	svc := newReverseFixture(&fakeReverser{})
	svc.Adapters[domain.SourceMarkdown] = errAdapter{}
	rep, err := svc.Reverse(context.Background(), []string{"A-1"})
	if err != nil {
		t.Fatal(err)
	}
	if r := rep.Results[0]; r.Status != domain.ReverseError || !strings.Contains(r.Detail, "disk") {
		t.Fatalf("snapshot の失敗は error: %+v", r)
	}
}

func TestReverse_NoAdapterIsSkipped(t *testing.T) {
	svc := newReverseFixture(&fakeReverser{})
	delete(svc.Adapters, domain.SourceMarkdown)
	rep, err := svc.Reverse(context.Background(), []string{"A-1"})
	if err != nil {
		t.Fatal(err)
	}
	if r := rep.Results[0]; r.Status != domain.ReverseSkipped {
		t.Fatalf("adapter が無ければ skipped: %+v", r)
	}
}

func TestReverseReport_Failed(t *testing.T) {
	cases := []struct {
		name   string
		status []domain.ReverseStatus
		strict bool
		want   bool
	}{
		{"既定では mismatch でも失敗しない", []domain.ReverseStatus{domain.ReverseMismatch, domain.ReverseError}, false, false},
		{"strict で mismatch は失敗", []domain.ReverseStatus{domain.ReverseMatch, domain.ReverseMismatch}, true, true},
		{"strict で error も失敗（判定できていない）", []domain.ReverseStatus{domain.ReverseError}, true, true},
		{"strict でも skipped は失敗しない", []domain.ReverseStatus{domain.ReverseMatch, domain.ReverseSkipped}, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rep ReverseReport
			for _, s := range tc.status {
				rep.Results = append(rep.Results, domain.ReverseResult{Status: s})
			}
			if got := rep.Failed(tc.strict); got != tc.want {
				t.Fatalf("Failed(%v) = %v", tc.strict, got)
			}
		})
	}
}

type errAdapter struct{}

func (errAdapter) Type() domain.SourceType { return domain.SourceMarkdown }
func (errAdapter) Snapshot(context.Context, string) (domain.Snapshot, error) {
	return domain.Snapshot{}, errors.New("disk error")
}
func (errAdapter) Units(context.Context, string) ([]string, error) { return nil, nil }
