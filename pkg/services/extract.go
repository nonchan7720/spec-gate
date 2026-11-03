package services

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// ExtractService は同じ仕様から独立に N 回 claim を抽出し、一致した主張だけを高信頼とする（合議）。
// 出力は claims/_pending/ に置かれ、人間が承認するまで check の対象にならない。
type ExtractService struct {
	Claims    domain.ClaimRepository
	Adapters  map[domain.SourceType]domain.SourceAdapter
	Extractor domain.Extractor
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

	type bucket struct {
		claim domain.Claim
		hits  int
	}
	buckets := map[string]*bucket{}
	for i := range runs {
		claims, err := s.Extractor.Extract(ctx, ref, snap.Excerpt)
		if err != nil {
			return nil, fmt.Errorf("抽出 %d/%d 回目: %w", i+1, runs, err)
		}
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
				buckets[key] = &bucket{claim: c, hits: 1}
			}
		}
	}

	var out []Candidate
	for _, b := range buckets {
		conf := fmt.Sprintf("%d/%d", b.hits, runs)
		b.claim.Approved = nil
		b.claim.Source.Type = srcType
		p, err := s.Claims.SavePending(b.claim, conf)
		if err != nil {
			return nil, err
		}
		out = append(out, Candidate{Claim: b.claim, Agreements: b.hits, Runs: runs, Path: p})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Agreements != out[j].Agreements {
			return out[i].Agreements > out[j].Agreements
		}
		return out[i].Claim.ID < out[j].Claim.ID
	})
	return out, nil
}

var nonWord = regexp.MustCompile(`[\s　、。,.「」()（）【】]+`)

// fingerprint は「同じ主張」を同定するキー。kind + 正規化した title + 例の入力集合。
// 完全ではないが、表記揺れ程度なら同一とみなせる。
func fingerprint(c domain.Claim) string {
	t := strings.ToLower(nonWord.ReplaceAllString(c.Title, ""))
	t = strings.Split(t, "要確認")[0]
	var ins []string
	for _, e := range c.Examples {
		ins = append(ins, fmt.Sprint(e.In))
	}
	sort.Strings(ins)
	return string(c.Kind) + "|" + t + "|" + strings.Join(ins, ",")
}
