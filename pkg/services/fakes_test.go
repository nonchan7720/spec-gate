package services

import (
	"context"
	"errors"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

type fakeClaims struct {
	claims []domain.Claim
	saved  []domain.Claim
	err    error
}

func (f *fakeClaims) All() ([]domain.Claim, error) { return f.claims, f.err }
func (f *fakeClaims) Save(c domain.Claim) error {
	f.saved = append(f.saved, c)
	return nil
}
func (f *fakeClaims) SavePending(domain.Claim, string) (string, error) { return "", nil }

// fakeAdapter は ref ごとの snapshot を返す。
type fakeAdapter struct {
	typ   domain.SourceType
	snaps map[string]domain.Snapshot
	units map[string][]string
}

func (f fakeAdapter) Type() domain.SourceType { return f.typ }
func (f fakeAdapter) Snapshot(_ context.Context, ref string) (domain.Snapshot, error) {
	return f.snaps[ref], nil
}
func (f fakeAdapter) Units(_ context.Context, ref string) ([]string, error) {
	u, ok := f.units[ref]
	if !ok {
		return nil, errors.New("no such source")
	}
	return u, nil
}

// fakeChecker は claim ID ごとに決めた結果を返す。
type fakeChecker struct {
	kind    domain.ClaimKind
	results map[string]domain.Status
	opts    []domain.CheckOptions
}

func (f *fakeChecker) Kind() domain.ClaimKind { return f.kind }
func (f *fakeChecker) Check(_ context.Context, c domain.Claim, opt domain.CheckOptions) domain.Result {
	f.opts = append(f.opts, opt)
	return domain.Result{ID: c.ID, Title: c.Title, Status: f.results[c.ID]}
}

// fakeRunner は "false" で始まるコマンドだけ失敗させる。
type fakeRunner struct{ calls []string }

func (f *fakeRunner) Run(_ context.Context, command string) (string, error) {
	f.calls = append(f.calls, command)
	if strings.HasPrefix(command, "false") {
		return "boom", errors.New("exit status 1")
	}
	return "", nil
}

type fakeChanges struct {
	files []string
	err   error
}

func (f fakeChanges) ChangedFiles() ([]string, error) { return f.files, f.err }

type fixedClock string

func (c fixedClock) Now() string { return string(c) }

type fakeSources []domain.SourceEntry

func (f fakeSources) All() ([]domain.SourceEntry, error) { return f, nil }

func approved(hash string) *domain.Approval {
	return &domain.Approval{By: "alice", At: "2026-11-01T00:00:00Z", SourceHash: hash}
}

func mdClaim(id, ref string, kind domain.ClaimKind, a *domain.Approval) domain.Claim {
	return domain.Claim{ID: id, Kind: kind, Title: id, Source: domain.Source{Type: domain.SourceMarkdown, Ref: ref}, Approved: a, Binding: "npx tsx tools/call.ts src/round.ts f", Examples: []domain.Example{{In: 1, Out: 1}}}
}
