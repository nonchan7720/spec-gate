package services

import (
	"sort"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// TraceService は claim がどう通ったか（通り方のログ）を記録・要約する。
// 何度も fail した末に大量の変更で pass した、のような無理やり通した形跡を人間が見られるようにする。
type TraceService struct {
	Log   domain.TraceLog
	Lines domain.LineCounter
	Clock domain.Clock
}

// Record は claim の判定を 1 件 1 行で追記する。変更行数が取れなければ 0 として記録を続ける。
func (s *TraceService) Record(rs domain.Results) error {
	lines, err := s.Lines.ChangedLines()
	if err != nil {
		lines = 0
	}
	now := s.Clock.Now()
	entries := make([]domain.TraceEntry, 0, len(rs))
	for _, r := range rs {
		entries = append(entries, domain.TraceEntry{Time: now, ID: r.ID, Status: r.Status, Lines: lines})
	}
	return s.Log.Append(entries)
}

// ClaimTrace は claim 1 件の通り方。
type ClaimTrace struct {
	ID string
	// Runs は記録された判定の回数
	Runs int
	// Last は最後の判定
	Last domain.Status
	// Pass は最後に pass になったときの通り方。pass していなければ nil
	Pass *domain.TraceSummary
}

// Summary は記録を claim ごとにまとめ、id 順に返す。
func (s *TraceService) Summary() ([]ClaimTrace, error) {
	entries, err := s.Log.All()
	if err != nil {
		return nil, err
	}
	byID := map[string][]domain.TraceEntry{}
	for _, e := range entries {
		byID[e.ID] = append(byID[e.ID], e)
	}
	out := make([]ClaimTrace, 0, len(byID))
	for id, h := range byID {
		out = append(out, ClaimTrace{ID: id, Runs: len(h), Last: h[len(h)-1].Status, Pass: domain.SummarizeTrace(h)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
