package services

import (
	"fmt"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// PlanService はプランの verify 群を凍結する。凍結後の verify 変更は check で stale になる。
type PlanService struct {
	Plans domain.PlanRepository
	Clock domain.Clock
}

func (s *PlanService) Freeze(path, by string) (domain.Plan, error) {
	p, err := s.Plans.Load(path)
	if err != nil {
		return p, err
	}
	if missing := p.MissingVerify(); len(missing) > 0 {
		var lines []string
		for _, m := range missing {
			lines = append(lines, fmt.Sprintf("  L%d: %s", m.Line, m.Text))
		}
		return p, fmt.Errorf("verify のない項目があるため凍結できない:\n%s", strings.Join(lines, "\n"))
	}
	if len(p.Items) == 0 {
		return p, fmt.Errorf("チェック項目がない")
	}
	return p, s.Plans.Freeze(path, domain.PlanLock{Hash: p.VerifyHash(), FrozenAt: s.Clock.Now(), FrozenBy: by})
}

// CheckFrozen は凍結済みプランの verify 行が凍結時から変わっていないかを確かめる。
// 未凍結のプランは編集自由（nil）。編集直後の hook から呼び、verify の書き換えをその場で止める。
func (s *PlanService) CheckFrozen(path string) error {
	p, err := s.Plans.Load(path)
	if err != nil {
		return err
	}
	lock, err := s.Plans.Lock(path)
	if err != nil {
		return err
	}
	if lock == nil {
		return nil
	}
	if lock.Hash != p.VerifyHash() {
		return fmt.Errorf("%s: verify 行が凍結（%s, %s）後に変更された。verify は元に戻し、変更が必要なら人間に相談する", path, lock.FrozenAt, lock.FrozenBy)
	}
	return nil
}

// StatusService は claim の一覧（判定は走らせない）。
type StatusService struct {
	Claims domain.ClaimRepository
	Plans  domain.PlanRepository
}

type StatusRow struct {
	Claim    domain.Claim
	Approved bool
}

func (s *StatusService) List() ([]StatusRow, []domain.Plan, error) {
	claims, err := s.Claims.All()
	if err != nil {
		return nil, nil, err
	}
	rows := make([]StatusRow, 0, len(claims))
	for _, c := range claims {
		rows = append(rows, StatusRow{Claim: c, Approved: c.Approved != nil})
	}
	plans, err := s.Plans.All()
	return rows, plans, err
}
