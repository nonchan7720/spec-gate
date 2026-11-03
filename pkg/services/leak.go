package services

import "github.com/nonchan7720/spec-gate/pkg/domain"

// LeakService は実装ファイルに claim の examples・holdout の値がそのまま書かれていないかを調べる
// （docs/spec/harness.md「リテラル漏れ検知」）。ファイルの読み込みは呼び出し側が行う。
type LeakService struct {
	Claims domain.ClaimRepository
	// Holdout が nil なら holdout の値は調べない。
	Holdout domain.HoldoutRepository
}

// LeakFile は調べるファイル 1 つ。Path はプロジェクトルート相対（対象外の判定に使う）。
type LeakFile struct {
	Path    string
	Content string
}

// LeakFinding は見つかった値 1 件。
// holdout だけにある値は Hidden にし、Value を持たない（エージェントに holdout の値を確かめさせない）。
type LeakFinding struct {
	File   string
	Line   int
	Value  any
	Hidden bool
}

type LeakReport struct {
	Findings []LeakFinding
	// Skipped は対象外（テスト・claims/・holdout/・docs/）で調べなかったファイル。
	Skipped []string
}

func (s *LeakService) Scan(files []LeakFile) (LeakReport, error) {
	public, hidden, err := s.values()
	if err != nil {
		return LeakReport{}, err
	}
	var rep LeakReport
	for _, f := range files {
		if !domain.LeakTarget(f.Path) {
			rep.Skipped = append(rep.Skipped, f.Path)
			continue
		}
		for _, l := range domain.FindLeaks(f.Content, public) {
			rep.Findings = append(rep.Findings, LeakFinding{File: f.Path, Line: l.Line, Value: l.Value})
		}
		for _, l := range domain.FindLeaks(f.Content, hidden) {
			rep.Findings = append(rep.Findings, LeakFinding{File: f.Path, Line: l.Line, Hidden: true})
		}
	}
	return rep, nil
}

// values は claim の examples の値（公開）と、holdout だけにある値（非公開）を返す。
// examples にも同じ値があれば公開として扱う（表示しても holdout は漏れない）。
func (s *LeakService) values() (public, hidden []any, err error) {
	claims, err := s.Claims.All()
	if err != nil {
		return nil, nil, err
	}
	for _, c := range claims {
		public = append(public, domain.ExampleValues(c.Examples)...)
	}
	if s.Holdout == nil {
		return public, nil, nil
	}
	all, err := s.Holdout.All()
	if err != nil {
		return nil, nil, err
	}
	for _, exs := range all {
		for _, v := range domain.ExampleValues(exs) {
			if !containsValue(public, v) {
				hidden = append(hidden, v)
			}
		}
	}
	return public, hidden, nil
}

func containsValue(vs []any, v any) bool {
	for _, x := range vs {
		if domain.Equal(x, v) {
			return true
		}
	}
	return false
}
