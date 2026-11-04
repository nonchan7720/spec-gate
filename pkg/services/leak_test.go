package services

import (
	"errors"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

func leakService() *LeakService {
	c := mdClaim("A", "s.md#a", domain.KindExample, nil)
	c.Examples = []domain.Example{{In: 1234.56, Out: 1234}, {In: "円未満は切り捨て", Out: 1}}
	return &LeakService{
		Claims:  &fakeClaims{claims: []domain.Claim{c}},
		Holdout: fakeHoldout{by: map[string][]domain.Example{"A": {{In: 31337.5, Out: 1234}}}},
	}
}

func TestLeak_FindsExampleAndHoldoutValues(t *testing.T) {
	rep, err := leakService().Scan([]LeakFile{
		{Path: "src/round.ts", Content: "// 円未満は切り捨て\nif (x === 31337.5) return 1234\n"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []LeakFinding{
		{File: "src/round.ts", Line: 2, Value: 1234.0},
		{File: "src/round.ts", Line: 1, Value: "円未満は切り捨て"},
		{File: "src/round.ts", Line: 2, Hidden: true},
	}
	if len(rep.Findings) != len(want) {
		t.Fatalf("got %+v", rep.Findings)
	}
	for i := range want {
		if rep.Findings[i] != want[i] {
			t.Fatalf("[%d] got %+v, want %+v", i, rep.Findings[i], want[i])
		}
	}
}

func TestLeak_SkipsExcludedFiles(t *testing.T) {
	rep, err := leakService().Scan([]LeakFile{
		{Path: "src/round.test.ts", Content: "expect(f(1234.56)).toBe(1234)"},
		{Path: "docs/spec/rounding.md", Content: "1234.56 → 1234"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Findings) != 0 || len(rep.Skipped) != 2 {
		t.Fatalf("テスト・docs は対象外: %+v", rep)
	}
}

func TestLeak_HoldoutErrorIsReturned(t *testing.T) {
	s := leakService()
	s.Holdout = fakeHoldout{err: errors.New("holdout/A.yaml を読めない")}
	if _, err := s.Scan([]LeakFile{{Path: "src/x.ts", Content: ""}}); err == nil {
		t.Fatal("holdout が読めなければエラー")
	}
}
