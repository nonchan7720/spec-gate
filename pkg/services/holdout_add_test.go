package services

import (
	"errors"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

type appended struct {
	id string
	ex domain.HoldoutExample
}

type fakeHoldoutWriter struct {
	got []appended
	err error
}

func (f *fakeHoldoutWriter) Append(id string, ex domain.HoldoutExample) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.got = append(f.got, appended{id, ex})
	return "holdout/" + id + ".yaml", nil
}

func TestHoldoutAdd_WritesHashOnly(t *testing.T) {
	w := &fakeHoldoutWriter{}
	s := &HoldoutService{Claims: &fakeClaims{claims: []domain.Claim{mdClaim("A", "s.md#a", domain.KindExample, nil)}}, Writer: w}
	path, err := s.Add("A", 31337.5, 1234)
	if err != nil {
		t.Fatal(err)
	}
	if path != "holdout/A.yaml" || len(w.got) != 1 {
		t.Fatalf("1 件だけ足す: %s %+v", path, w.got)
	}
	if w.got[0].id != "A" || !domain.Equal(w.got[0].ex.In, 31337.5) || w.got[0].ex.OutSHA256 != hash1234 {
		t.Fatalf("in は平文、期待値は正規化 JSON の SHA-256 だけを書く: %+v", w.got[0])
	}
}

func TestHoldoutAdd_RejectsUnknownOrNonExampleClaim(t *testing.T) {
	w := &fakeHoldoutWriter{}
	s := &HoldoutService{Claims: &fakeClaims{claims: []domain.Claim{mdClaim("M", "s.md#a", domain.KindManual, nil)}}, Writer: w}
	for _, id := range []string{"NONE", "M"} {
		if _, err := s.Add(id, 1, 2); err == nil {
			t.Fatalf("%s: claim が無い・kind: example でない claim には足さない", id)
		}
	}
	if len(w.got) != 0 {
		t.Fatalf("書かない: %+v", w.got)
	}
}

func TestHoldoutAdd_WriterErrorHidesOutput(t *testing.T) {
	s := &HoldoutService{
		Claims: &fakeClaims{claims: []domain.Claim{mdClaim("A", "s.md#a", domain.KindExample, nil)}},
		Writer: &fakeHoldoutWriter{err: errors.New("書けない")},
	}
	_, err := s.Add("A", 1, 4242)
	if err == nil || strings.Contains(err.Error(), "4242") {
		t.Fatalf("書けなければエラー（期待値は出さない）: %v", err)
	}
}
