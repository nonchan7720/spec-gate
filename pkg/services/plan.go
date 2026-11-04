package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// PlanService はプランの verify 群を凍結する。凍結後の verify 変更は check で stale になる。
type PlanService struct {
	Plans domain.PlanRepository
	Clock domain.Clock
	// Mutation は front matter に mutation（閾値）があるプランの凍結前に score を計算する。
	Mutation *MutationService
	// MutationMax は凍結時に走らせる変異の上限（0 以下なら DefaultMutationMax）。
	MutationMax int
}

// Freeze は verify 行を凍結する。front matter に mutation があれば、mutation score が閾値未満のとき拒否する。
// score を計算したときはその結果も返す（閾値が無ければ nil）。
func (s *PlanService) Freeze(ctx context.Context, path, by string) (domain.Plan, *domain.MutationResult, error) {
	p, err := s.Plans.Load(path)
	if err != nil {
		return p, nil, err
	}
	if missing := p.MissingVerify(); len(missing) > 0 {
		var lines []string
		for _, m := range missing {
			lines = append(lines, fmt.Sprintf("  L%d: %s", m.Line, m.Text))
		}
		return p, nil, fmt.Errorf("verify のない項目があるため凍結できない:\n%s", strings.Join(lines, "\n"))
	}
	if len(p.Items) == 0 {
		return p, nil, fmt.Errorf("チェック項目がない")
	}
	var res *domain.MutationResult
	if p.Mutation != nil {
		r, err := s.mutationGate(ctx, p)
		if err != nil {
			return p, r, err
		}
		res = r
	}
	return p, res, s.Plans.Freeze(path, domain.PlanLock{Hash: p.VerifyHash(), FrozenAt: s.Clock.Now(), FrozenBy: by})
}

// mutationGate は mutation score を計算し、閾値未満・計算不能なら凍結を拒否するエラーを返す。
func (s *PlanService) mutationGate(ctx context.Context, p domain.Plan) (*domain.MutationResult, error) {
	th := *p.Mutation
	if s.Mutation == nil {
		return nil, fmt.Errorf("mutation: %v があるが mutation score を計算できないため凍結しない", th)
	}
	max := s.MutationMax
	if max <= 0 {
		max = DefaultMutationMax
	}
	r, err := s.Mutation.Score(ctx, p, max)
	if err != nil {
		return nil, fmt.Errorf("mutation score を計算できないため凍結しない: %w", err)
	}
	if r.Total == 0 {
		return &r, fmt.Errorf("mutation: %v があるが、claims が触るファイルに変異を 1 件も作れない（score を出せない）ため凍結しない", th)
	}
	if score := r.Score(); score < th {
		return &r, fmt.Errorf("mutation score %.2f（%d/%d 件を検出）が閾値 %v 未満のため凍結しない。verify を強くする。検出できなかった変異:\n%s",
			score, r.Killed, r.Total, th, FormatSurvivors(r.Survivors))
	}
	return &r, nil
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
