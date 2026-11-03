// Package services はユースケース。ドメインとポートだけに依存し、具体的な I/O は infrastructure が担う。
package services

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/internal/textutil"
)

// CheckService は claim とプランを判定する。
type CheckService struct {
	Claims   domain.ClaimRepository
	Plans    domain.PlanRepository
	Adapters map[domain.SourceType]domain.SourceAdapter
	Checkers map[domain.ClaimKind]domain.Checker
	Runner   domain.CommandRunner
	Changes  domain.ChangeDetector
	// DiagramEdges は structure の from-diagram 用に、mermaid ref からエッジ集合を返す。
	DiagramEdges func(ref string) (map[string]bool, error)
}

type CheckRequest struct {
	Only    []string // claim ID の絞り込み
	Changed bool     // 変更ファイルに関係する claim だけ
	UI      bool     // Playwright を実行
	NoPlans bool     // プランを見ない
}

type CheckReport struct {
	Claims domain.Results
	Plans  []PlanReport
	// Skipped は --changed で対象外になった claim 数。
	Skipped int
}

type PlanReport struct {
	Path  string
	Title string
	Items domain.Results
}

func (r CheckReport) HasBlocking() bool {
	if r.Claims.HasBlocking() {
		return true
	}
	for _, p := range r.Plans {
		if p.Items.HasBlocking() {
			return true
		}
	}
	return false
}

func (s *CheckService) Run(ctx context.Context, req CheckRequest) (CheckReport, error) {
	claims, err := s.Claims.All()
	if err != nil {
		return CheckReport{}, err
	}
	claims, skipped, err := s.filter(ctx, claims, req)
	if err != nil {
		return CheckReport{}, err
	}

	report := CheckReport{Skipped: skipped}
	for _, c := range claims {
		report.Claims = append(report.Claims, s.checkOne(ctx, c, req))
	}

	if !req.NoPlans && len(req.Only) == 0 {
		plans, err := s.Plans.All()
		if err != nil {
			return report, err
		}
		for _, p := range plans {
			if !p.Active() {
				continue
			}
			report.Plans = append(report.Plans, s.checkPlan(ctx, p))
		}
	}
	return report, nil
}

// checkOne: 承認 → 仕様ドリフト → 種類別チェック の順で、最初に引っかかった段階で止める。
func (s *CheckService) checkOne(ctx context.Context, c domain.Claim, req CheckRequest) domain.Result {
	res := domain.Result{ID: c.ID, Title: c.Title}

	if c.Approved == nil {
		res.Status = domain.StatusUnapproved
		res.Detail = fmt.Sprintf("`spec-gate approve %s` で承認する", c.ID)
		return res
	}

	snap, err := s.snapshot(ctx, c.Source)
	if err != nil {
		res.Status = domain.StatusFail
		res.Detail = err.Error()
		return res
	}
	res.Spec = snap.Location
	switch {
	case snap.Skipped != "" && c.Source.DriftRequired():
		res.Status = domain.StatusFail
		res.Detail = "仕様を読めない: " + snap.Skipped
		return res
	case snap.Skipped == "" && snap.Hash != c.Approved.SourceHash:
		res.Status = domain.StatusStale
		res.Detail = fmt.Sprintf("仕様が承認（%s）後に変更された。内容を確認して `spec-gate approve %s` で再承認する\n--- 現在の仕様 ---\n%s", c.Approved.At, c.ID, snap.Excerpt)
		return res
	}

	if c.Kind == domain.KindManual {
		res.Status = domain.StatusUnverified
		res.Detail = "manual: 機械判定なし。人間のレビュー対象"
		return res
	}
	chk, ok := s.Checkers[c.Kind]
	if !ok {
		res.Status = domain.StatusFail
		res.Detail = fmt.Sprintf("kind %s の checker が未登録", c.Kind)
		return res
	}
	opt := domain.CheckOptions{UI: req.UI}
	if c.Kind == domain.KindStructure && c.Allowed == "from-diagram" && s.DiagramEdges != nil {
		edges, err := s.DiagramEdges(c.Source.Ref)
		if err != nil {
			res.Status = domain.StatusFail
			res.Detail = "図の読み込みに失敗: " + err.Error()
			return res
		}
		opt.DiagramEdges = edges
	}
	out := chk.Check(ctx, c, opt)
	out.Spec = snap.Location
	if snap.Skipped != "" && out.Status == domain.StatusPass {
		out.Detail = "注意: 仕様のドリフト検知は無効（" + snap.Skipped + "）"
	}
	return out
}

func (s *CheckService) checkPlan(ctx context.Context, p domain.Plan) PlanReport {
	rep := PlanReport{Path: p.Path, Title: p.Title}
	id := filepath.Base(p.Path)

	if missing := p.MissingVerify(); len(missing) > 0 {
		var lines []string
		for _, m := range missing {
			lines = append(lines, fmt.Sprintf("L%d: %s", m.Line, m.Text))
		}
		rep.Items = append(rep.Items, domain.Result{ID: id, Title: "verify のない項目", Status: domain.StatusFail, Detail: strings.Join(lines, "\n")})
		return rep
	}

	lock, err := s.Plans.Lock(p.Path)
	switch {
	case err != nil:
		rep.Items = append(rep.Items, domain.Result{ID: id, Title: "ロック読み込み", Status: domain.StatusFail, Detail: err.Error()})
		return rep
	case lock == nil:
		rep.Items = append(rep.Items, domain.Result{ID: id, Title: "未凍結", Status: domain.StatusUnapproved, Detail: fmt.Sprintf("`spec-gate plan freeze %s` で verify を凍結する", p.Path)})
		return rep
	case lock.Hash != p.VerifyHash():
		rep.Items = append(rep.Items, domain.Result{ID: id, Title: "verify が凍結後に変更された", Status: domain.StatusStale, Detail: "verify 行の変更は人間の再凍結が必要。コード側を直すこと"})
		return rep
	}

	for _, it := range p.Items {
		r := domain.Result{ID: fmt.Sprintf("%s:L%d", id, it.Line), Title: it.Text, Status: domain.StatusPass}
		out, err := s.Runner.Run(ctx, it.Verify)
		if err != nil {
			r.Status = domain.StatusFail
			r.Detail = fmt.Sprintf("$ %s\n%s", it.Verify, textutil.Tail(out, 15))
		}
		rep.Items = append(rep.Items, r)
	}
	return rep
}

func (s *CheckService) snapshot(ctx context.Context, src domain.Source) (domain.Snapshot, error) {
	ad, ok := s.Adapters[src.Type]
	if !ok {
		return domain.Snapshot{}, fmt.Errorf("source type %s の adapter が未登録", src.Type)
	}
	return ad.Snapshot(ctx, src.Ref)
}

// filter は --only / --changed を適用する。
func (s *CheckService) filter(ctx context.Context, claims []domain.Claim, req CheckRequest) ([]domain.Claim, int, error) {
	if len(req.Only) > 0 {
		want := map[string]bool{}
		for _, id := range req.Only {
			want[strings.TrimSpace(id)] = true
		}
		var out []domain.Claim
		for _, c := range claims {
			if want[c.ID] {
				out = append(out, c)
			}
		}
		return out, len(claims) - len(out), nil
	}
	if !req.Changed || s.Changes == nil {
		return claims, 0, nil
	}
	changed, err := s.Changes.ChangedFiles()
	if err != nil {
		return nil, 0, err
	}
	if changed == nil { // git でない
		return claims, 0, nil
	}
	var out []domain.Claim
	for _, c := range claims {
		if touched(c, changed) {
			out = append(out, c)
		}
	}
	return out, len(claims) - len(out), nil
}

func touched(c domain.Claim, changed []string) bool {
	srcFile, _ := splitRef(c.Source.Ref)
	paths := append(c.Touches(), srcFile, c.File)
	for _, ch := range changed {
		ch = filepath.ToSlash(ch)
		for _, p := range paths {
			p = strings.TrimPrefix(filepath.ToSlash(p), "./")
			if p == "" || strings.Contains(p, "://") {
				continue
			}
			if ch == p || strings.HasPrefix(ch, strings.TrimSuffix(p, "/")+"/") || strings.HasSuffix(p, ch) {
				return true
			}
		}
	}
	return false
}

func splitRef(ref string) (string, string) {
	if before, after, ok := strings.Cut(ref, "#"); ok {
		return before, after
	}
	return ref, ""
}
