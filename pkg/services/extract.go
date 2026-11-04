package services

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

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
}

type Candidate struct {
	Claim      domain.Claim
	Agreements int
	Runs       int
	Path       string
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
func (s *ExtractService) consensus(srcType domain.SourceType, perRun [][]domain.Claim) ([]Candidate, error) {
	runs := len(perRun)
	type bucket struct {
		claim domain.Claim
		hits  int
	}
	buckets := map[string]*bucket{}
	var order []*bucket // 初出順。ID の付け替えを決定的にする
	for _, claims := range perRun {
		seen := map[string]bool{}
		for _, c := range claims {
			key := fingerprint(c)
			if seen[key] {
				continue
			}
			seen[key] = true
			if b, ok := buckets[key]; ok {
				b.hits++
				if len(c.Examples) > len(b.claim.Examples) { // より情報量の多い方を残す
					b.claim = c
				}
			} else {
				b := &bucket{claim: c, hits: 1}
				buckets[key] = b
				order = append(order, b)
			}
		}
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
		out = append(out, Candidate{Claim: b.claim, Agreements: b.hits, Runs: runs, Path: p})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Agreements != out[j].Agreements {
			return out[i].Agreements > out[j].Agreements
		}
		return out[i].Claim.ID < out[j].Claim.ID
	})
	return out, nil
}

// fingerprint は「同じ主張」を同定するキー。
// 例があれば kind + (in, out) の組の集合で決める（title の言い回しは実行ごとに揺れるので見ない）。
// 例が無ければ kind + 正規化した title。
func fingerprint(c domain.Claim) string {
	if len(c.Examples) > 0 {
		set := map[string]bool{}
		for _, e := range c.Examples {
			set[fmt.Sprintf("%#v=>%#v", e.In, e.Out)] = true
		}
		pairs := make([]string, 0, len(set))
		for p := range set {
			pairs = append(pairs, p)
		}
		sort.Strings(pairs)
		return string(c.Kind) + "|ex|" + strings.Join(pairs, "\x00")
	}
	return string(c.Kind) + "|title|" + normalizeTitle(c.Title)
}

var (
	// 【要確認: …】 の注記。閉じ括弧が無ければ末尾まで。
	reviewNote = regexp.MustCompile(`【\s*要確認[^】]*(】|$)`)
	// [M-1] / (HOLDOUT-003) / 【R-2】 のような括弧つきの ID。
	bracketedID = regexp.MustCompile(`[\[(（【]\s*[A-Za-z][A-Za-z0-9_]*(-[A-Za-z0-9_]+)+\s*[\])）】]`)
)

// normalizeTitle は注記・括弧つき ID・句読点・記号・空白を落として小文字にする。
func normalizeTitle(t string) string {
	t = reviewNote.ReplaceAllString(t, "")
	t = strings.Split(t, "要確認")[0]
	t = bracketedID.ReplaceAllString(t, "")
	var b strings.Builder
	for _, r := range t {
		if unicode.IsPunct(r) || unicode.IsSymbol(r) || unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}
