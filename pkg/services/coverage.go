package services

import (
	"context"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// CoverageService は「仕様のどの部分が claim に紐づいていないか」を列挙する。
// 誤った claim より抜け落ちた claim の方が見つけにくいので、source 側から逆引きする。
type CoverageService struct {
	Claims   domain.ClaimRepository
	Sources  domain.SourceRegistry
	Adapters map[domain.SourceType]domain.SourceAdapter
}

type CoverageEntry struct {
	Source       string
	Covered      []string // claim に紐づく単位
	Uncovered    []string // 紐づかない規範的な単位
	NonNormative []string // 規範でないと宣言された単位
	Error        string
}

type CoverageReport struct {
	Entries []CoverageEntry
	// ByKind は claim の kind 別件数（検証手段の分布 = spec coverage）。
	ByKind map[domain.ClaimKind]int
	Total  int
}

func (s *CoverageService) Report(ctx context.Context) (CoverageReport, error) {
	claims, err := s.Claims.All()
	if err != nil {
		return CoverageReport{}, err
	}
	sources, err := s.Sources.All()
	if err != nil {
		return CoverageReport{}, err
	}

	rep := CoverageReport{ByKind: map[domain.ClaimKind]int{}, Total: len(claims)}
	refs := map[string]bool{}
	for _, c := range claims {
		rep.ByKind[c.Kind]++
		refs[c.Source.Ref] = true
	}

	for _, src := range sources {
		e := CoverageEntry{Source: src.RefOf()}
		ad, ok := s.Adapters[src.Type]
		if !ok {
			e.Error = "adapter 未登録: " + string(src.Type)
			rep.Entries = append(rep.Entries, e)
			continue
		}
		units, err := ad.Units(ctx, src.RefOf())
		if err != nil {
			e.Error = err.Error()
			rep.Entries = append(rep.Entries, e)
			continue
		}
		nonNorm := map[string]bool{}
		for _, n := range src.NonNormative {
			nonNorm[n] = true
		}
		for _, u := range units {
			_, anchor := splitRef(u)
			switch {
			case nonNorm[anchor]:
				e.NonNormative = append(e.NonNormative, u)
			case refs[u] || refs[src.RefOf()]: // ファイル全体を指す claim があれば全単位カバー扱い
				e.Covered = append(e.Covered, u)
			case src.Type == domain.SourceMermaid && coveredByStructure(claims, src.RefOf(), anchor):
				e.Covered = append(e.Covered, u)
			default:
				e.Uncovered = append(e.Uncovered, u)
			}
		}
		rep.Entries = append(rep.Entries, e)
	}
	return rep, nil
}

// mermaid のエッジは、その図を source にする structure claim の layers に両端が含まれていればカバー扱い。
func coveredByStructure(claims []domain.Claim, ref, edge string) bool {
	parts := strings.Split(edge, "->")
	if len(parts) != 2 {
		return false
	}
	from, to := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	for _, c := range claims {
		if c.Kind != domain.KindStructure || c.Source.Ref != ref {
			continue
		}
		if _, ok := c.Layers[from]; !ok {
			continue
		}
		if _, ok := c.Layers[to]; ok {
			return true
		}
	}
	return false
}
