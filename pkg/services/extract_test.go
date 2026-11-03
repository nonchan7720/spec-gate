package services

import (
	"context"
	"fmt"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// fakeExtractor は呼ばれるたびに runs[i] を返す。
type fakeExtractor struct {
	runs [][]domain.Claim
	n    int
}

func (f *fakeExtractor) Extract(context.Context, string, string) ([]domain.Claim, error) {
	if f.n >= len(f.runs) {
		return nil, fmt.Errorf("想定外の呼び出し %d", f.n+1)
	}
	c := f.runs[f.n]
	f.n++
	return c, nil
}

// pendingStore は SavePending された候補を記録する。
type pendingStore struct {
	fakeClaims
	pending []domain.Claim
	conf    []string
}

func (p *pendingStore) SavePending(c domain.Claim, conf string) (string, error) {
	p.pending = append(p.pending, c)
	p.conf = append(p.conf, conf)
	return fmt.Sprintf("claims/_pending/%d.yaml", len(p.pending)), nil
}

func runExtract(t *testing.T, runs ...[]domain.Claim) ([]Candidate, *pendingStore) {
	t.Helper()
	store := &pendingStore{}
	svc := &ExtractService{
		Claims: store,
		Adapters: map[domain.SourceType]domain.SourceAdapter{
			domain.SourceMarkdown: fakeAdapter{typ: domain.SourceMarkdown, snaps: map[string]domain.Snapshot{"a.md#x": {Excerpt: "spec"}}},
		},
		Extractor: &fakeExtractor{runs: runs},
	}
	cands, err := svc.Extract(context.Background(), domain.SourceMarkdown, "a.md#x", len(runs))
	if err != nil {
		t.Fatal(err)
	}
	return cands, store
}

func ex(id, title string, pairs ...[2]any) domain.Claim {
	c := domain.Claim{ID: id, Kind: domain.KindExample, Title: title, Source: domain.Source{Ref: "a.md#x"}}
	for _, p := range pairs {
		c.Examples = append(c.Examples, domain.Example{In: p[0], Out: p[1]})
	}
	return c
}

func manualClaim(id, title string) domain.Claim {
	return domain.Claim{ID: id, Kind: domain.KindManual, Title: title, Source: domain.Source{Ref: "a.md#x"}}
}

// 言い回しが違っても、例の (in, out) 集合が同じなら同じ主張として N/N に数える。
func TestExtract_ParaphrasedTitlesWithSameExamplesMerge(t *testing.T) {
	cands, store := runExtract(t,
		[]domain.Claim{ex("R-1", "端数は切り捨てる", [2]any{1.5, 1}, [2]any{2.7, 2})},
		[]domain.Claim{ex("R-1", "小数点以下を切り捨てて整数にする", [2]any{2.7, 2}, [2]any{1.5, 1})},
		[]domain.Claim{ex("R-7", "金額の端数処理は floor", [2]any{1.5, 1}, [2]any{2.7, 2}, [2]any{2.7, 2})},
	)
	if len(cands) != 1 || len(store.pending) != 1 {
		t.Fatalf("1 件にまとまる: %+v", cands)
	}
	if cands[0].Agreements != 3 || store.conf[0] != "3/3" {
		t.Fatalf("3/3 になる: %+v %v", cands[0], store.conf)
	}
}

// より情報量の多い候補（例が多い方）を残す。同じなら先に出たもの。
func TestExtract_KeepsMoreInformativeCandidate(t *testing.T) {
	cands, _ := runExtract(t,
		[]domain.Claim{ex("R-1", "切り捨て", [2]any{1.5, 1})},
		[]domain.Claim{ex("R-1", "切り捨て（詳細）", [2]any{1.5, 1})},
	)
	if len(cands) != 1 || cands[0].Agreements != 2 || cands[0].Claim.Title != "切り捨て" {
		t.Fatalf("同じ情報量なら最初のものを残す: %+v", cands)
	}

	cands, _ = runExtract(t,
		[]domain.Claim{ex("R-1", "切り捨て", [2]any{1.5, 1}, [2]any{2.5, 2})},
		[]domain.Claim{ex("R-1", "切り捨て（重複例あり）", [2]any{1.5, 1}, [2]any{2.5, 2}, [2]any{2.5, 2})},
	)
	if len(cands) != 1 || cands[0].Claim.Title != "切り捨て（重複例あり）" {
		t.Fatalf("例の多い方を残す: %+v", cands)
	}
}

// 例のない主張は title で同定するが、【要確認…】・句読点・括弧つきの ID は無視する。
func TestExtract_TitleNormalizationForClaimsWithoutExamples(t *testing.T) {
	cands, _ := runExtract(t,
		[]domain.Claim{manualClaim("M-1", "[M-1] 管理者だけが削除できる【要確認: 権限表が無い】")},
		[]domain.Claim{manualClaim("M-4", "管理者だけが、削除できる。")},
		[]domain.Claim{manualClaim("M-2", "(HOLDOUT-003) 管理者だけが削除できる!")},
	)
	if len(cands) != 1 || cands[0].Agreements != 3 {
		t.Fatalf("表記揺れはまとまる: %+v", cands)
	}
}

// 本当に違う主張はまとめない。
func TestExtract_DifferentClaimsStaySeparate(t *testing.T) {
	cands, _ := runExtract(t,
		[]domain.Claim{
			ex("R-1", "端数は切り捨てる", [2]any{1.5, 1}),
			ex("R-2", "端数は切り捨てる", [2]any{1.5, 2}), // out が違う
			ex("R-3", "端数は切り捨てる", [2]any{1.5, 1}, [2]any{3.5, 3}),
			manualClaim("M-1", "管理者だけが削除できる"),
			manualClaim("M-2", "一般ユーザーは閲覧だけできる"),
			{ID: "M-3", Kind: domain.KindUI, Title: "管理者だけが削除できる"},
		},
	)
	if len(cands) != 6 {
		t.Fatalf("6 件とも別の候補: %+v", cands)
	}
}

// 同じ ID の別の主張は、最初の 1 件を除いて ID を付け替える（承認時に衝突しない）。
func TestExtract_DistinctIDsForDifferentClaimsSharingAnID(t *testing.T) {
	_, store := runExtract(t,
		[]domain.Claim{
			manualClaim("HOLDOUT-003", "一つ目"),
			manualClaim("HOLDOUT-003", "二つ目"),
			manualClaim("HOLDOUT-003-2", "別の主張"),
		},
		[]domain.Claim{
			manualClaim("HOLDOUT-003", "三つ目"),
		},
	)
	if len(store.pending) != 4 {
		t.Fatalf("4 件とも残る: %+v", store.pending)
	}
	ids := map[string]string{}
	for _, c := range store.pending {
		if prev, dup := ids[c.ID]; dup {
			t.Fatalf("ID %s が重複: %s と %s", c.ID, prev, c.Title)
		}
		ids[c.ID] = c.Title
	}
	if ids["HOLDOUT-003"] != "一つ目" || ids["HOLDOUT-003-2"] != "別の主張" {
		t.Fatalf("最初の 1 件と元から一意な ID はそのまま: %v", ids)
	}
}
