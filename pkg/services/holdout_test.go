package services

import (
	"context"
	"errors"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

type fakeHoldout struct {
	by  map[string][]domain.Example
	err error
}

func (f fakeHoldout) Holdout(id string) ([]domain.Example, error) { return f.by[id], f.err }
func (f fakeHoldout) All() (map[string][]domain.Example, error)   { return f.by, f.err }

func TestCheck_PassesHoldoutToChecker(t *testing.T) {
	snaps := map[string]domain.Snapshot{"s.md#a": {Hash: "h1"}}
	claims := []domain.Claim{mdClaim("A", "s.md#a", domain.KindExample, approved("h1"))}
	s, chk := newCheck(claims, snaps, map[string]domain.Status{"A": domain.StatusPass})
	s.Holdout = fakeHoldout{by: map[string][]domain.Example{"A": {{In: 2, Out: 2}}}}

	if _, err := s.Run(context.Background(), CheckRequest{NoPlans: true}); err != nil {
		t.Fatal(err)
	}
	if len(chk.opts) != 1 || len(chk.opts[0].Holdout) != 1 {
		t.Fatalf("checker に holdout を渡す: %+v", chk.opts)
	}
}

func TestCheck_HoldoutReadErrorFails(t *testing.T) {
	snaps := map[string]domain.Snapshot{"s.md#a": {Hash: "h1"}}
	claims := []domain.Claim{mdClaim("A", "s.md#a", domain.KindExample, approved("h1"))}
	s, chk := newCheck(claims, snaps, map[string]domain.Status{"A": domain.StatusPass})
	s.Holdout = fakeHoldout{err: errors.New("holdout/A.yaml を読めない")}

	rep, err := s.Run(context.Background(), CheckRequest{NoPlans: true})
	if err != nil {
		t.Fatal(err)
	}
	if r := statusOf(t, rep, "A"); r.Status != domain.StatusFail {
		t.Fatalf("holdout が読めなければ検証していないのと同じなので fail: %+v", r)
	}
	if len(chk.opts) != 0 {
		t.Fatal("holdout が読めないまま checker を走らせない")
	}
}

func TestCheck_HoldoutChangeTouchesClaim(t *testing.T) {
	c := mdClaim("A", "s.md#a", domain.KindExample, approved("h1"))
	if !touched(c, []string{"holdout/A.yaml"}, nil, nil) {
		t.Fatal("holdout/<id>.yaml の変更はその claim を対象にする")
	}
	if touched(c, []string{"holdout/B.yaml"}, nil, nil) {
		t.Fatal("別の claim の holdout は対象外")
	}
}
