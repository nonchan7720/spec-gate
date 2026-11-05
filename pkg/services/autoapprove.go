package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// AutoApproveService は仕様 1 か所について、抽出から承認までを 1 つのコマンドの中で行う（docs/spec/auto-approval.md「自動承認」）。
// LLM の呼び出しは方針（policy.yaml）の llm_commands だけ。途中の回答はファイルに書かない。
type AutoApproveService struct {
	Policy   domain.AutoPolicySource
	Adapters map[domain.SourceType]domain.SourceAdapter
	LLM      domain.LLMRunner
	Extract  domain.ExtractPrompter
	Reverse  domain.ReversePrompter
	Probe    domain.BindingProbe
	Claims   domain.ClaimRepository
	Placer   domain.AutoPlacer
	Evidence domain.EvidenceStore
	// Decisions はブレーカーの入力（過去の自動承認と差し戻し）、Log は自動承認の記録
	Decisions domain.ReviewDecisionReader
	Log       domain.ReviewDecisionLog
	Clock     domain.Clock
}

// AutoOutcome は候補 1 件の結果。
type AutoOutcome struct {
	ID         string
	Title      string
	Verdict    string
	Confidence string
	DryRun     string
	Reverse    string
	// Path は置いたファイル（ok なら claims/<area>/、それ以外は claims/_pending/）
	Path string
}

// AutoReport は auto-approve 1 回分の結果。
type AutoReport struct {
	Paused   bool
	Outcomes []AutoOutcome
}

// ErrAutoDisabled は方針で自動承認が有効になっていないとき。
var ErrAutoDisabled = errors.New("自動承認は無効（.spec-gate/policy.yaml の auto_approve.enabled が true でない）")

// Run は ref の仕様から候補を作り、判定して、ok なら claims/<area>/ に承認済みで置き、それ以外は _pending に置く。
func (s *AutoApproveService) Run(ctx context.Context, srcType domain.SourceType, ref, area string) (AutoReport, error) {
	f, raw, err := s.Policy.Load()
	if err != nil {
		return AutoReport{}, err
	}
	policy, err := domain.ResolveAutoPolicy(f)
	if err != nil {
		return AutoReport{}, err
	}
	if !policy.Enabled {
		return AutoReport{}, ErrAutoDisabled
	}
	ad, ok := s.Adapters[srcType]
	if !ok {
		return AutoReport{}, fmt.Errorf("source type %s の adapter が未登録", srcType)
	}
	snap, err := ad.Snapshot(ctx, ref)
	if err != nil {
		return AutoReport{}, err
	}
	if snap.Skipped != "" {
		return AutoReport{}, fmt.Errorf("仕様を読めない: %s", snap.Skipped)
	}

	// 1. 方針のコマンドで runs 回抽出し、合議する
	commands := domain.AutoRunCommands(policy.LLMCommands, policy.Runs)
	perRun := make([][]domain.Claim, 0, len(commands))
	runs := make([]domain.EvidenceRun, 0, len(commands))
	for i, cmd := range commands {
		answer, err := s.LLM.Run(ctx, cmd, s.Extract.Prompt(ref, snap.Excerpt))
		if err != nil {
			return AutoReport{}, fmt.Errorf("抽出 %d/%d 回目: %w", i+1, len(commands), err)
		}
		runs = append(runs, domain.EvidenceRun{Command: cmd, AnswerSHA256: domain.SHA256Ref([]byte(answer))})
		claims, err := s.Extract.Parse(answer, ref)
		if err != nil {
			claims = nil // 読めない回答は、その run が何も主張しなかったものとして数える（合意数に表れる）
		}
		perRun = append(perRun, claims)
	}
	cands := mergeConsensus(srcType, perRun)

	paused, err := s.paused(policy)
	if err != nil {
		return AutoReport{}, err
	}
	existing, err := s.Claims.All()
	if err != nil {
		return AutoReport{}, err
	}
	have := map[string]bool{}
	for _, c := range existing {
		have[c.ID] = true
	}

	rep := AutoReport{Paused: paused}
	for _, cand := range cands {
		o, err := s.judge(ctx, policy, paused, cand, snap, existing, have, commands[0], area, raw, runs)
		if err != nil {
			return rep, err
		}
		rep.Outcomes = append(rep.Outcomes, o)
	}
	return rep, nil
}

// paused は記録の自動承認と差し戻しからブレーカーが止めているかを返す。
func (s *AutoApproveService) paused(p domain.AutoPolicy) (bool, error) {
	ds, err := s.Decisions.All()
	if err != nil {
		return false, err
	}
	hist := domain.BreakerHistory(domain.AutoApprovals(ds), p.Breaker.ResetAt)
	return domain.AutoBreakerTripped(p.Breaker.Window, p.Breaker.MaxRevoked, hist), nil
}

// judge は候補 1 件を判定して置き、証拠を書く。
func (s *AutoApproveService) judge(ctx context.Context, p domain.AutoPolicy, paused bool, cand Candidate, snap domain.Snapshot,
	existing []domain.Claim, have map[string]bool, reverseCmd, area string, policyRaw []byte, runs []domain.EvidenceRun) (AutoOutcome, error) {
	c := cand.Claim
	o := AutoOutcome{ID: c.ID, Title: c.Title, Confidence: cand.Confidence(), Reverse: "skipped"}
	if s.Probe != nil && c.NeedsDryRun() {
		o.DryRun = s.Probe.Probe(ctx, c).Summary()
	}
	ac := domain.AutoCandidate{Kind: c.Kind, Title: c.Title, Confidence: o.Confidence, DryRun: o.DryRun, Reverse: "match"}
	// 逆変換は判定の最後の条件なので、それより前で止まる候補には LLM を呼ばない
	o.Verdict = domain.AutoEligibility(p, paused, ac)
	if o.Verdict == domain.AutoOK && have[c.ID] {
		o.Verdict = domain.AutoExists
	}
	var diffs []string
	if o.Verdict == domain.AutoOK && p.RequireReverse {
		o.Reverse, diffs = s.reverse(ctx, reverseCmd, c, snap.Excerpt)
		ac.Reverse = o.Reverse
		o.Verdict = domain.AutoEligibility(p, paused, ac)
	}
	if o.Verdict == domain.AutoOK && lintFails(existing, c) {
		o.Verdict = domain.AutoLint
	}

	// 既にある ID の候補は、判定にかかわらず証拠を書かない（既にある claim の証拠を上書きすると、その claim が unapproved になる）
	var sha string
	if !have[c.ID] {
		ev := domain.Evidence{ID: c.ID, Ref: c.Source.Ref, SourceHash: snap.Hash, PolicySHA256: domain.SHA256Ref(policyRaw), Runs: runs,
			Confidence: o.Confidence, Reverse: o.Reverse, Differences: diffs, DryRun: o.DryRun, Verdict: o.Verdict, At: s.Clock.Now()}
		var err error
		if sha, err = s.Evidence.Save(ev); err != nil {
			return o, err
		}
	}

	if o.Verdict != domain.AutoOK {
		path, err := s.Placer.SavePendingAuto(c, o.Confidence, o.DryRun, o.Verdict)
		o.Path = path
		return o, err
	}
	c.Approved = &domain.Approval{By: domain.AutoApproverName, At: s.Clock.Now(), SourceHash: snap.Hash, Evidence: sha}
	path, err := s.Placer.PlaceApproved(c, area)
	if err != nil {
		return o, err
	}
	o.Path = path
	entry := domain.ReviewDecision{Time: s.Clock.Now(), ID: c.ID, Verdict: domain.VerdictApprove, By: domain.AutoApproverName, Evidence: domain.EvidenceRelPath(c.ID)}
	if err := s.Log.Append(entry); err != nil {
		return o, fmt.Errorf("%s: 承認したが記録に失敗: %w", c.ID, err)
	}
	return o, nil
}

// reverse は claim だけから仕様文を書き起こし、元の仕様と比べる。LLM の失敗や読めない回答は error。
func (s *AutoApproveService) reverse(ctx context.Context, cmd string, c domain.Claim, original string) (string, []string) {
	prompt, err := s.Reverse.RestatePrompt(c)
	if err != nil {
		return "error", []string{err.Error()}
	}
	answer, err := s.LLM.Run(ctx, cmd, prompt)
	if err != nil {
		return "error", []string{err.Error()}
	}
	restated, err := s.Reverse.ParseRestate(answer)
	if err != nil {
		return "error", []string{err.Error()}
	}
	answer, err = s.LLM.Run(ctx, cmd, s.Reverse.ComparePrompt(restated, original))
	if err != nil {
		return "error", []string{err.Error()}
	}
	cmp, err := s.Reverse.ParseCompare(answer)
	if err != nil {
		return "error", []string{err.Error()}
	}
	if cmp.Match {
		return "match", nil
	}
	return "mismatch", cmp.Differences
}

// lintFails は既存の claim に c を足したとき、c について lint の指摘があるか。
func lintFails(existing []domain.Claim, c domain.Claim) bool {
	for _, is := range domain.Lint(append(append([]domain.Claim(nil), existing...), c)) {
		if is.ClaimID == c.ID {
			return true
		}
	}
	return false
}
