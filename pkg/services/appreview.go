package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// MCP Apps の承認 UI（docs/spec/harness.md「MCP Apps の承認 UI」）。
// 候補の内容を UI に見せ、UI から返ってきた判断を、見せた内容の sha256 が今の候補と一致するときだけ反映する。
// 反映は承認画面・リモート承認と同じ ReviewService の処理を使う。

// AppItem は UI に出す候補 1 件。
type AppItem struct {
	Name       string `json:"name"`
	ID         string `json:"id"`
	Title      string `json:"title"`
	Kind       string `json:"kind"`
	Confidence string `json:"confidence,omitempty"`
	DryRun     string `json:"dry_run,omitempty"`
	Hint       string `json:"hint,omitempty"`
	// Eligible はまとめ承認の対象か（domain.BulkEligible）
	Eligible bool `json:"eligible"`
	// Preview は候補の内容（domain.RenderRemotePreview）。ContentSHA256 はその sha256（16 進）
	Preview       string `json:"preview"`
	ContentSHA256 string `json:"content_sha256"`
	SpecExcerpt   string `json:"spec_excerpt,omitempty"`
	SpecError     string `json:"spec_error,omitempty"`
	// Error は読めない候補の理由（承認できない。却下だけできる）
	Error string `json:"error,omitempty"`
}

// AppDecision は UI からの判断 1 件。
type AppDecision struct {
	Name          string               `json:"name"`
	Verdict       domain.RemoteVerdict `json:"verdict"`
	Reason        string               `json:"reason,omitempty"`
	ContentSHA256 string               `json:"content_sha256"`
}

func contentSHA(preview string) string {
	sum := sha256.Sum256([]byte(preview))
	return hex.EncodeToString(sum[:])
}

// AppItems は承認待ち候補を承認しやすい順に、内容と仕様の抜粋つきで返す。
func (s *RemoteReviewService) AppItems(ctx context.Context) ([]AppItem, error) {
	items, err := s.Review.List(ctx)
	if err != nil {
		return nil, err
	}
	ids := map[string]int{}
	for _, it := range items {
		ids[it.Claim.ID]++
	}
	// 承認したときに lint で拒否される候補は、画面で先に理由を見せ、まとめ承認の対象から外す
	var approved []domain.Claim
	if s.Review.Approver != nil && s.Review.Approver.Claims != nil {
		if approved, err = s.Review.Approver.Claims.All(); err != nil {
			return nil, err
		}
	}
	out := make([]AppItem, 0, len(items))
	for _, it := range items {
		preview := domain.RenderRemotePreview(it.PendingClaim)
		a := AppItem{
			Name: it.Name, ID: it.Claim.ID, Title: it.Claim.Title, Kind: string(it.Claim.Kind),
			Confidence: it.Confidence, DryRun: it.DryRun, Hint: it.Claim.Hint,
			Eligible: ids[it.Claim.ID] == 1 && domain.BulkEligible(it.PendingClaim, 0),
			Preview:  preview, ContentSHA256: contentSHA(preview),
			SpecExcerpt: it.Spec.Excerpt, SpecError: it.SpecError, Error: it.Error,
		}
		if ids[it.Claim.ID] > 1 && a.Error == "" {
			a.Error = fmt.Sprintf("候補 ID %s が複数のファイルにある（承認すると衝突する）", it.Claim.ID)
		}
		if a.Error == "" {
			if msg := lintFor(approved, it.Claim); msg != "" {
				a.Error = "承認すると lint で拒否される: " + msg
			}
		}
		if a.Error != "" {
			a.Eligible = false
		}
		out = append(out, a)
	}
	return out, nil
}

// current は候補名 → 今の内容（preview）。
func (s *RemoteReviewService) current() (map[string]domain.PendingClaim, error) {
	s.Review.mu.Lock()
	pending, err := s.Review.Pending.Pending()
	s.Review.mu.Unlock()
	if err != nil {
		return nil, err
	}
	m := make(map[string]domain.PendingClaim, len(pending))
	for _, p := range pending {
		m[p.Name] = p
	}
	return m, nil
}

// ErrContentChanged は UI に見せた内容と今の候補が違うときのエラー。
var ErrContentChanged = errors.New("表示した内容と今の候補が一致しないので反映しない（画面を読み直してから判断する）")

// AppDecide は判断 1 件を反映し、記録する。見せた内容の sha256 が今の候補と違えば反映しない。
func (s *RemoteReviewService) AppDecide(ctx context.Context, d AppDecision, by string) error {
	cur, err := s.current()
	if err != nil {
		return err
	}
	p, ok := cur[d.Name]
	if !ok {
		return fmt.Errorf("候補 %s がない", d.Name)
	}
	preview := domain.RenderRemotePreview(p)
	if d.ContentSHA256 != contentSHA(preview) {
		return ErrContentChanged
	}
	rd := domain.RemoteDecision{ID: p.Claim.ID, Verdict: d.Verdict, Reason: d.Reason}
	if d.Verdict == domain.VerdictApprove && p.Error != "" {
		return fmt.Errorf("読めない候補は承認できない（却下だけできる）: %s", p.Error)
	}
	if err := s.apply(ctx, rd, d.Name, by); err != nil {
		return err
	}
	return s.Log.Append(domain.ReviewDecision{Time: s.Clock.Now(), ID: rd.ID, Verdict: rd.Verdict, Reason: rd.Reason, By: by, ContentSHA256: d.ContentSHA256})
}

// AppBulkApprove は候補をまとめて承認する。見せた内容が今の候補と違うものは承認しない。
// example は承認の前に試し実行し直し、pass でなければ承認しない（ReviewService.BulkApprove）。
func (s *RemoteReviewService) AppBulkApprove(ctx context.Context, ds []AppDecision, by string) []BulkOutcome {
	cur, err := s.current()
	if err != nil {
		return []BulkOutcome{{Error: err.Error()}}
	}
	var names []string
	var out []BulkOutcome
	shown := map[string]string{}
	for _, d := range ds {
		p, ok := cur[d.Name]
		switch {
		case !ok:
			out = append(out, BulkOutcome{Name: d.Name, Error: "候補がない"})
		case d.ContentSHA256 != contentSHA(domain.RenderRemotePreview(p)):
			out = append(out, BulkOutcome{Name: d.Name, ID: p.Claim.ID, Error: ErrContentChanged.Error()})
		default:
			names = append(names, d.Name)
			shown[d.Name] = d.ContentSHA256
		}
	}
	for _, o := range s.Review.BulkApprove(ctx, names, by, true) {
		if o.Error == "" {
			if err := s.Log.Append(domain.ReviewDecision{Time: s.Clock.Now(), ID: o.ID, Verdict: domain.VerdictApprove, By: by, ContentSHA256: shown[o.Name]}); err != nil {
				o.Error = "承認したが記録に失敗: " + err.Error()
			}
		}
		out = append(out, o)
	}
	return out
}

// lintFor は claim を承認済みの claim に加えたときの、その claim の lint の指摘（無ければ ""）。
func lintFor(approved []domain.Claim, c domain.Claim) string {
	var msgs []string
	for _, is := range domain.Lint(append(append([]domain.Claim{}, approved...), c)) {
		if is.ClaimID == c.ID {
			msgs = append(msgs, is.Message)
		}
	}
	return strings.Join(msgs, " / ")
}
