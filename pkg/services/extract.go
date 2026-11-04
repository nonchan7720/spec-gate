package services

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// ExtractService は同じ仕様から独立に N 回 claim を抽出し、一致した主張だけを高信頼とする（合議）。
// 出力は claims/_pending/ に置かれ、人間が承認するまで check の対象にならない。
type ExtractService struct {
	Claims   domain.ClaimRepository
	Adapters map[domain.SourceType]domain.SourceAdapter
	// Extractor は LLM を直接呼ぶ抽出（SPEC_GATE_EXTRACT_CMD のコマンドモード）。委譲（Emit / Collect）では使わない
	Extractor domain.Extractor
	// Prompter と Work は委譲（Emit / Collect）で使う
	Prompter domain.ExtractPrompter
	Work     domain.LLMWorkspace
	// Probe と Pending があれば、置いた example 候補を試し実行して `# dry-run:` を書く（docs/spec/harness.md「抽出結果の試し実行」）
	Probe   domain.BindingProbe
	Pending domain.PendingRepository
}

type Candidate struct {
	Claim      domain.Claim
	Agreements int
	Runs       int
	Path       string
	// DryRun は試し実行の結果（`# dry-run:` に書いた文字列）。試していなければ空
	DryRun string
}

func (s *ExtractService) Extract(ctx context.Context, srcType domain.SourceType, ref string, runs int) ([]Candidate, error) {
	ad, ok := s.Adapters[srcType]
	if !ok {
		return nil, fmt.Errorf("source type %s の adapter が未登録", srcType)
	}
	snap, err := ad.Snapshot(ctx, ref)
	if err != nil {
		return nil, err
	}
	if snap.Skipped != "" {
		return nil, fmt.Errorf("仕様を読めない: %s", snap.Skipped)
	}

	perRun := make([][]domain.Claim, 0, runs)
	for i := range runs {
		claims, err := s.Extractor.Extract(ctx, ref, snap.Excerpt)
		if err != nil {
			return nil, fmt.Errorf("抽出 %d/%d 回目: %w", i+1, runs, err)
		}
		perRun = append(perRun, claims)
	}
	return s.consensus(srcType, perRun)
}

// consensus は run ごとに独立に抽出した claim の配列を合議し、claims/_pending/ に置く。
// 同じ主張の同定（言い換えを含む）は domain.MergeRuns に従う。
func (s *ExtractService) consensus(srcType domain.SourceType, perRun [][]domain.Claim) ([]Candidate, error) {
	runs := len(perRun)
	type bucket struct {
		claim domain.Claim
		hits  int
	}
	var order []*bucket // 初出順。ID の付け替えを決定的にする
	for _, a := range domain.MergeRuns(perRun) {
		order = append(order, &bucket{claim: a.Claim, hits: a.Hits})
	}

	// LLM は節・実行ごとに ID を振るので、別の主張が同じ ID を持つことがある。
	// 最初の 1 件はそのまま、以降は -2, -3 … を付けて一意にする（承認時に衝突させない）。
	used := map[string]bool{}
	for _, b := range order {
		used[b.claim.ID] = true
	}
	taken := map[string]bool{}
	for _, b := range order {
		id := b.claim.ID
		if taken[id] {
			for n := 2; ; n++ {
				cand := fmt.Sprintf("%s-%d", id, n)
				if !used[cand] && !taken[cand] {
					id = cand
					break
				}
			}
		}
		taken[id] = true
		b.claim.ID = id
	}

	var out []Candidate
	for _, b := range order {
		conf := fmt.Sprintf("%d/%d", b.hits, runs)
		b.claim.Approved = nil
		b.claim.Source.Type = srcType
		p, err := s.Claims.SavePending(b.claim, conf)
		if err != nil {
			return nil, err
		}
		cand := Candidate{Claim: b.claim, Agreements: b.hits, Runs: runs, Path: p}
		if s.Probe != nil && s.Pending != nil && b.claim.NeedsDryRun() {
			cand.DryRun = s.Probe.Probe(context.Background(), b.claim).Summary()
			if err := s.Pending.WriteDryRun(filepath.Base(p), cand.DryRun); err != nil {
				return nil, err
			}
		}
		out = append(out, cand)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Agreements != out[j].Agreements {
			return out[i].Agreements > out[j].Agreements
		}
		return out[i].Claim.ID < out[j].Claim.ID
	})
	return out, nil
}
