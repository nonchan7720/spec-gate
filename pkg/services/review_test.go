package services

import (
	"context"
	"errors"
	"path"
	"sort"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// fakeStore は claims/ と claims/_pending/ をメモリ上で再現する（ClaimRepository と PendingRepository の両方）。
type fakeStore struct {
	approved []domain.Claim
	pending  map[string]domain.PendingClaim
	saved    []domain.Claim
	demoted  []string
	// ambiguous は曖昧として記録した候補名 → 理由
	ambiguous map[string]string
}

func (f *fakeStore) All() ([]domain.Claim, error) { return f.approved, nil }
func (f *fakeStore) Save(c domain.Claim) error {
	f.saved = append(f.saved, c)
	return nil
}
func (f *fakeStore) SavePending(domain.Claim, string) (string, error) { return "", nil }

func (f *fakeStore) Pending() ([]domain.PendingClaim, error) {
	var out []domain.PendingClaim
	for _, p := range f.pending {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (f *fakeStore) Promote(name, area string) (string, error) {
	p, ok := f.pending[name]
	if !ok {
		return "", errors.New("候補がない")
	}
	delete(f.pending, name)
	c := p.Claim
	c.File = path.Join("claims", area, name)
	f.approved = append(f.approved, c)
	return c.File, nil
}

func (f *fakeStore) Demote(p string) error {
	for i, c := range f.approved {
		if c.File == p {
			f.approved = append(f.approved[:i], f.approved[i+1:]...)
			name := path.Base(p)
			c.File = path.Join("claims/_pending", name)
			f.pending[name] = domain.PendingClaim{Name: name, Claim: c}
			f.demoted = append(f.demoted, p)
			return nil
		}
	}
	return errors.New("not found")
}

func (f *fakeStore) DiscardPending(name string) error {
	if _, ok := f.pending[name]; !ok {
		return errors.New("候補がない")
	}
	delete(f.pending, name)
	return nil
}

func newReview(pending []domain.PendingClaim, snaps map[string]domain.Snapshot) (*ReviewService, *fakeStore) {
	store := &fakeStore{pending: map[string]domain.PendingClaim{}}
	for _, p := range pending {
		store.pending[p.Name] = p
	}
	adapters := map[domain.SourceType]domain.SourceAdapter{domain.SourceMarkdown: fakeAdapter{typ: domain.SourceMarkdown, snaps: snaps}}
	return &ReviewService{
		Pending:  store,
		Adapters: adapters,
		Approver: &ApproveService{Claims: store, Adapters: adapters, Clock: fixedClock("2026-11-04T00:00:00Z")},
	}, store
}

func pending(c domain.Claim, confidence string) domain.PendingClaim {
	return domain.PendingClaim{Name: c.ID + ".yaml", Path: "claims/_pending/" + c.ID + ".yaml", Confidence: confidence, Claim: c}
}

func TestAreaFor(t *testing.T) {
	cases := map[domain.ClaimKind]string{
		domain.KindExample:   "spec",
		domain.KindManual:    "spec",
		domain.KindStructure: "arch",
		domain.KindUI:        "ui",
	}
	for k, want := range cases {
		if got := AreaFor(k); got != want {
			t.Errorf("AreaFor(%s) = %q, want %q", k, got, want)
		}
	}
}

func TestReviewList_WithSpecExcerpt(t *testing.T) {
	ok := mdClaim("A", "s.md#a", domain.KindExample, nil)
	unreadable := mdClaim("U", "s.md#missing", domain.KindExample, nil)
	noAdapter := mdClaim("F", "figma://x/1", domain.KindUI, nil)
	noAdapter.Source.Type = domain.SourceFigma
	broken := domain.PendingClaim{Name: "B.yaml", Error: "title がない"}
	s, _ := newReview([]domain.PendingClaim{pending(ok, "3/3"), pending(unreadable, "1/3"), pending(noAdapter, "2/3"), broken},
		map[string]domain.Snapshot{"s.md#a": {Hash: "h1", Excerpt: "仕様の本文"}, "s.md#missing": {Skipped: "見出しが無い"}})

	items, err := s.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]ReviewItem{}
	for _, it := range items {
		byName[it.Name] = it
	}
	if len(items) != 4 {
		t.Fatalf("壊れた候補も含めて全件: %+v", items)
	}
	if a := byName["A.yaml"]; a.Area != "spec" || a.Spec.Excerpt != "仕様の本文" || a.Confidence != "3/3" || a.SpecError != "" {
		t.Fatalf("移動先の領域と仕様の抜粋が付く: %+v", a)
	}
	if u := byName["U.yaml"]; !strings.Contains(u.SpecError, "見出しが無い") {
		t.Fatalf("仕様を読めない理由が付く: %+v", u)
	}
	if f := byName["F.yaml"]; f.Area != "ui" || !strings.Contains(f.SpecError, "adapter") {
		t.Fatalf("adapter が無ければその旨: %+v", f)
	}
	if b := byName["B.yaml"]; b.Error == "" || b.Area != "" {
		t.Fatalf("壊れた候補はエラーのまま・領域なし: %+v", b)
	}
}

func TestReviewApprove_MovesThenApproves(t *testing.T) {
	c := mdClaim("A", "s.md#a", domain.KindExample, nil)
	s, store := newReview([]domain.PendingClaim{pending(c, "3/3")}, map[string]domain.Snapshot{"s.md#a": {Hash: "h1", Excerpt: "仕様の本文"}})

	res, err := s.Approve(context.Background(), "A.yaml", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(store.saved) != 1 || store.saved[0].File != "claims/spec/A.yaml" || store.saved[0].Approved.By != "alice" || store.saved[0].Approved.SourceHash != "h1" {
		t.Fatalf("claims/spec/ に移してから承認を記録: %+v", store.saved)
	}
	if res.Excerpt != "仕様の本文" || len(store.pending) != 0 {
		t.Fatalf("承認後は _pending から消える: %+v %+v", res, store.pending)
	}
}

func TestReviewApprove_RollsBackOnRejection(t *testing.T) {
	existing := mdClaim("A", "s.md#a", domain.KindExample, approved("h1"))
	existing.Examples = []domain.Example{{In: 10, Out: 10}}
	conflict := mdClaim("B", "s.md#a", domain.KindExample, nil)
	conflict.Examples = []domain.Example{{In: 10, Out: 11}}
	unreadable := mdClaim("U", "s.md#missing", domain.KindExample, nil)

	cases := []struct {
		name, file, want string
	}{
		{"lint に失敗", "B.yaml", "lint に失敗"},
		{"仕様が読めない", "U.yaml", "仕様を読めない"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, store := newReview([]domain.PendingClaim{pending(conflict, "2/3"), pending(unreadable, "2/3")},
				map[string]domain.Snapshot{"s.md#a": {Hash: "h1"}, "s.md#missing": {Skipped: "見出しが無い"}})
			existing.File = "claims/spec/A.yaml"
			store.approved = []domain.Claim{existing}

			_, err := s.Approve(context.Background(), tc.file, "alice")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			if len(store.saved) != 0 {
				t.Fatalf("拒否したら保存しない: %+v", store.saved)
			}
			if _, ok := store.pending[tc.file]; !ok || len(store.demoted) != 1 {
				t.Fatalf("拒否されたら _pending に戻す: %+v %+v", store.pending, store.demoted)
			}
			if len(store.approved) != 1 {
				t.Fatalf("領域には残さない: %+v", store.approved)
			}
		})
	}
}

func TestReviewApprove_RefusesBrokenOrMissing(t *testing.T) {
	broken := domain.PendingClaim{Name: "B.yaml", Error: "title がない"}
	s, store := newReview([]domain.PendingClaim{broken}, nil)
	if _, err := s.Approve(context.Background(), "B.yaml", "alice"); err == nil || !strings.Contains(err.Error(), "title がない") {
		t.Fatalf("壊れた候補は承認できない: %v", err)
	}
	if _, ok := store.pending["B.yaml"]; !ok {
		t.Fatal("壊れた候補は移さない")
	}
	if _, err := s.Approve(context.Background(), "NOPE.yaml", "alice"); err == nil || !strings.Contains(err.Error(), "がない") {
		t.Fatalf("無い候補はエラー: %v", err)
	}
}

func TestReviewReject(t *testing.T) {
	c := mdClaim("A", "s.md#a", domain.KindExample, nil)
	s, store := newReview([]domain.PendingClaim{pending(c, "1/3")}, nil)
	if err := s.Reject("A.yaml"); err != nil {
		t.Fatal(err)
	}
	if len(store.pending) != 0 {
		t.Fatalf("却下で消える: %+v", store.pending)
	}
	if err := s.Reject("A.yaml"); err == nil {
		t.Fatal("無い候補はエラー")
	}
}
