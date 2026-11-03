package services

import "github.com/nonchan7720/spec-gate/pkg/domain"

// LintService は claim の内部整合を検査する。examples に加え、holdout も properties との整合を確かめる。
// holdout の問題は番号だけで報告し、値は出さない（domain.LintHoldout）。
type LintService struct {
	Claims domain.ClaimRepository
	// Holdout が nil なら holdout は見ない。
	Holdout domain.HoldoutRepository
}

// Run は lint の指摘と、検査した claim の件数を返す。
func (s *LintService) Run() ([]domain.LintIssue, int, error) {
	claims, err := s.Claims.All()
	if err != nil {
		return nil, 0, err
	}
	issues := domain.Lint(claims)
	if s.Holdout != nil {
		holdout, err := s.Holdout.All()
		if err != nil {
			return nil, 0, err
		}
		issues = append(issues, domain.LintHoldout(claims, holdout)...)
	}
	return issues, len(claims), nil
}
