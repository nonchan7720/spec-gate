package services

import (
	"errors"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

type fakePlans struct {
	plans map[string]domain.Plan
	locks map[string]*domain.PlanLock
}

func (f *fakePlans) All() ([]domain.Plan, error) {
	var out []domain.Plan
	for _, p := range f.plans {
		out = append(out, p)
	}
	return out, nil
}

func (f *fakePlans) Load(path string) (domain.Plan, error) {
	p, ok := f.plans[path]
	if !ok {
		return domain.Plan{}, errors.New("not found")
	}
	return p, nil
}

func (f *fakePlans) Lock(path string) (*domain.PlanLock, error) { return f.locks[path], nil }

func (f *fakePlans) Freeze(path string, lock domain.PlanLock) error {
	f.locks[path] = &lock
	return nil
}

func planWith(path string, verifies ...string) domain.Plan {
	p := domain.Plan{Path: path}
	for i, v := range verifies {
		p.Items = append(p.Items, domain.PlanItem{Line: i + 1, Text: "item", Verify: v})
	}
	return p
}

func TestPlanCheckFrozen_NotFrozenIsOK(t *testing.T) {
	repo := &fakePlans{plans: map[string]domain.Plan{"plans/a.md": planWith("plans/a.md", "go test ./...")}, locks: map[string]*domain.PlanLock{}}
	s := &PlanService{Plans: repo}
	if err := s.CheckFrozen("plans/a.md"); err != nil {
		t.Fatalf("未凍結のプランは自由に編集できるべき: %v", err)
	}
}

func TestPlanCheckFrozen_UnchangedVerifyIsOK(t *testing.T) {
	p := planWith("plans/a.md", "go test ./...")
	repo := &fakePlans{plans: map[string]domain.Plan{"plans/a.md": p}, locks: map[string]*domain.PlanLock{"plans/a.md": {Hash: p.VerifyHash()}}}
	s := &PlanService{Plans: repo}
	if err := s.CheckFrozen("plans/a.md"); err != nil {
		t.Fatalf("verify が凍結時のままなら通すべき: %v", err)
	}
}

func TestPlanCheckFrozen_ChangedVerifyFails(t *testing.T) {
	frozen := planWith("plans/a.md", "go test ./...")
	edited := planWith("plans/a.md", "true")
	repo := &fakePlans{plans: map[string]domain.Plan{"plans/a.md": edited}, locks: map[string]*domain.PlanLock{"plans/a.md": {Hash: frozen.VerifyHash()}}}
	s := &PlanService{Plans: repo}
	err := s.CheckFrozen("plans/a.md")
	if err == nil || !strings.Contains(err.Error(), "凍結") {
		t.Fatalf("凍結後の verify 変更を検出すべき: %v", err)
	}
}

func TestPlanCheckFrozen_LoadErrorFails(t *testing.T) {
	repo := &fakePlans{plans: map[string]domain.Plan{}, locks: map[string]*domain.PlanLock{}}
	s := &PlanService{Plans: repo}
	if err := s.CheckFrozen("plans/missing.md"); err == nil {
		t.Fatal("読めないプランはエラーにすべき")
	}
}
