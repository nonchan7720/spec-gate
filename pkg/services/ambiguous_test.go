package services

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// MarkAmbiguous は fakeStore を PendingRepository として完成させる（曖昧の記録）。
func (f *fakeStore) MarkAmbiguous(name, reason string) (string, error) {
	if _, ok := f.pending[name]; !ok {
		return "", errors.New("候補がない")
	}
	delete(f.pending, name)
	if f.ambiguous == nil {
		f.ambiguous = map[string]string{}
	}
	f.ambiguous[name] = reason
	return "claims/_ambiguous/" + name, nil
}

func TestReviewMarkAmbiguous(t *testing.T) {
	c := mdClaim("A", "s.md#a", domain.KindExample, nil)
	s, store := newReview([]domain.PendingClaim{pending(c, "2/3")}, nil)
	if err := s.MarkAmbiguous("A.yaml", "  "); err == nil || !strings.Contains(err.Error(), "理由") {
		t.Fatalf("理由が空なら記録しない: %v", err)
	}
	if _, ok := store.pending["A.yaml"]; !ok {
		t.Fatal("理由が空なら移さない")
	}
	if err := s.MarkAmbiguous("A.yaml", "締め日の扱いが決まっていない"); err != nil {
		t.Fatal(err)
	}
	if len(store.pending) != 0 || store.ambiguous["A.yaml"] != "締め日の扱いが決まっていない" {
		t.Fatalf("候補を _ambiguous に移して理由を記録する: pending=%+v ambiguous=%+v", store.pending, store.ambiguous)
	}
	if err := s.MarkAmbiguous("A.yaml", "x"); err == nil || !strings.Contains(err.Error(), "がない") {
		t.Fatalf("無い候補はエラー: %v", err)
	}
}

func TestReviewMarkAmbiguous_BrokenCandidateIsAllowed(t *testing.T) {
	// 壊れた候補でも「仕様が決まらないので書けなかった」ことはありうる。記録は止めない
	broken := domain.PendingClaim{Name: "B.yaml", Error: "title がない"}
	s, store := newReview([]domain.PendingClaim{broken}, nil)
	if err := s.MarkAmbiguous("B.yaml", "金額の上限が未定"); err != nil {
		t.Fatal(err)
	}
	if store.ambiguous["B.yaml"] == "" {
		t.Fatal("記録される")
	}
}

type fakeAmbiguous []domain.AmbiguousClaim

func (f fakeAmbiguous) Ambiguous() ([]domain.AmbiguousClaim, error) { return f, nil }

func TestCoverage_AmbiguousUnitsNeedReview(t *testing.T) {
	claims := []domain.Claim{mdClaim("R", "spec.md#端数処理", domain.KindExample, nil)}
	md := fakeAdapter{typ: domain.SourceMarkdown, units: map[string][]string{
		"spec.md":  {"spec.md#端数処理", "spec.md#月末処理", "spec.md#休日"},
		"other.md": {"other.md#a", "other.md#b"},
	}}
	amb := fakeAmbiguous{
		{Name: "M.yaml", Reason: "月末が休日のときが未定", Claim: domain.Claim{ID: "M", Source: domain.Source{Type: domain.SourceMarkdown, Ref: "spec.md#月末処理"}}},
		{Name: "R2.yaml", Reason: "端数の向きが未定", Claim: domain.Claim{ID: "R2", Source: domain.Source{Type: domain.SourceMarkdown, Ref: "spec.md#端数処理"}}},
		{Name: "W.yaml", Reason: "全体が未定", Claim: domain.Claim{ID: "W", Source: domain.Source{Type: domain.SourceMarkdown, Ref: "other.md"}}},
		{Name: "X.yaml", Error: "壊れている"},
	}
	s := &CoverageService{
		Claims:    &fakeClaims{claims: claims},
		Ambiguous: amb,
		Sources:   fakeSources{{Type: domain.SourceMarkdown, Path: "spec.md"}, {Type: domain.SourceMarkdown, Path: "other.md"}},
		Adapters:  map[domain.SourceType]domain.SourceAdapter{domain.SourceMarkdown: md},
	}
	rep, err := s.Report(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	e := rep.Entries[0]
	want := []NeedsReview{
		{Unit: "spec.md#端数処理", Reasons: []string{"R2: 端数の向きが未定"}},
		{Unit: "spec.md#月末処理", Reasons: []string{"M: 月末が休日のときが未定"}},
	}
	if !reflect.DeepEqual(e.NeedsReview, want) {
		t.Errorf("曖昧として記録された claim の参照する単位は要確認（承認済み claim があっても）: %+v", e.NeedsReview)
	}
	if len(e.Covered) != 0 || !reflect.DeepEqual(e.Uncovered, []string{"spec.md#休日"}) {
		t.Errorf("要確認の単位は covered / uncovered に重ねて出さない: %+v", e)
	}
	o := rep.Entries[1]
	if len(o.NeedsReview) != 2 || len(o.Uncovered) != 0 {
		t.Errorf("ファイル全体を指す曖昧 claim なら全単位が要確認: %+v", o)
	}
}

// WriteDryRun は試し実行の結果を候補に記録する。
func (f *fakeStore) WriteDryRun(name, summary string) error {
	p, ok := f.pending[name]
	if !ok {
		return errors.New("候補がない")
	}
	p.DryRun = summary
	f.pending[name] = p
	return nil
}
