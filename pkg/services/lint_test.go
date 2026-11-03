package services

import (
	"errors"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

func lintClaim() domain.Claim {
	c := mdClaim("A", "s.md#a", domain.KindExample, nil)
	c.Properties = []string{"output <= input"}
	return c
}

func TestLint_IncludesHoldoutWithoutValues(t *testing.T) {
	s := &LintService{
		Claims:  &fakeClaims{claims: []domain.Claim{lintClaim()}},
		Holdout: fakeHoldout{by: map[string][]domain.Example{"A": {{In: 31337, Out: 42424}}}},
	}
	issues, n, err := s.Run()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || len(issues) != 1 || issues[0].ClaimID != "A" {
		t.Fatalf("holdout が property を満たさなければ報告する: %d %+v", n, issues)
	}
	if strings.Contains(issues[0].Message, "31337") || strings.Contains(issues[0].Message, "42424") {
		t.Fatalf("holdout の値は出さない: %q", issues[0].Message)
	}
}

func TestLint_WithoutHoldoutRepository(t *testing.T) {
	s := &LintService{Claims: &fakeClaims{claims: []domain.Claim{lintClaim()}}}
	issues, n, err := s.Run()
	if err != nil || n != 1 || len(issues) != 0 {
		t.Fatalf("holdout が無ければ examples だけを見る: %v %d %+v", err, n, issues)
	}
}

func TestLint_HoldoutReadError(t *testing.T) {
	s := &LintService{
		Claims:  &fakeClaims{claims: []domain.Claim{lintClaim()}},
		Holdout: fakeHoldout{err: errors.New("holdout/A.yaml を読めない")},
	}
	if _, _, err := s.Run(); err == nil {
		t.Fatal("holdout が読めなければエラー（黙って通さない）")
	}
}
