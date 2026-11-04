package services

import "github.com/nonchan7720/spec-gate/pkg/domain"

// Kinds は claim の種類を表示順に並べたもの。
var Kinds = []domain.ClaimKind{domain.KindExample, domain.KindStructure, domain.KindUI, domain.KindManual}

// KindMetric は claim の種類 1 つの件数と、総数に対する割合。
type KindMetric struct {
	Kind  domain.ClaimKind `json:"kind"`
	Count int              `json:"count"`
	Ratio float64          `json:"ratio"`
}

// Metrics は CI で推移を見るための claim の指標。
type Metrics struct {
	Total int `json:"total"`
	// ByKind は Kinds の順に、0 件の種類も含めて並べる
	ByKind         []KindMetric `json:"by_kind"`
	Unverified     int          `json:"unverified"`
	UnverifiedRate float64      `json:"unverified_rate"`
	// ManualRate は manual の claim の割合（docs/spec/harness.md「manual の手がかり」）
	ManualRate float64 `json:"manual_rate"`
}

// ComputeMetrics は claim の判定結果から指標を出す。
func ComputeMetrics(rs domain.Results) Metrics {
	m := Metrics{Total: len(rs), Unverified: rs.Count(domain.StatusUnverified), UnverifiedRate: domain.UnverifiedRate(rs.Statuses())}
	count := map[domain.ClaimKind]int{}
	kinds := make([]domain.ClaimKind, 0, len(rs))
	for _, r := range rs {
		count[r.Kind]++
		kinds = append(kinds, r.Kind)
	}
	m.ManualRate = domain.ManualRate(kinds)
	for _, k := range Kinds {
		km := KindMetric{Kind: k, Count: count[k]}
		if m.Total > 0 {
			km.Ratio = float64(km.Count) / float64(m.Total)
		}
		m.ByKind = append(m.ByKind, km)
	}
	return m
}
