package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// RemoteAskMax は 1 回の質問ダイアログに出せる質問（候補）の上限。
const RemoteAskMax = 4

// RemoteReviewService はリモート承認（質問ダイアログで候補を見せ、hook で受けた回答を反映する）。
// 反映は ReviewService の承認・却下・曖昧の記録をそのまま使う。
type RemoteReviewService struct {
	Review *ReviewService
	Log    domain.ReviewDecisionLog
	Clock  domain.Clock
}

// RemoteOutcome は判定 1 件を反映した結果。Error が空なら反映できた。
type RemoteOutcome struct {
	Decision domain.RemoteDecision
	// Name は候補のファイル名
	Name  string
	Error string
}

// RemoteRecordResult は hook 1 回分の結果。Notes は反映しなかった質問や記録の失敗の理由。
type RemoteRecordResult struct {
	Outcomes []RemoteOutcome
	Notes    []string
}

// candidates は承認待ち候補を ID ごとにまとめる。読めない候補と ID が重複する候補は外し、理由を notes に返す。
// order は承認しやすい順（domain.SortForReview）の ID。
func (s *RemoteReviewService) candidates() (byID map[string]domain.PendingClaim, order, notes []string, err error) {
	s.Review.mu.Lock()
	pending, err := s.Review.Pending.Pending()
	s.Review.mu.Unlock()
	if err != nil {
		return nil, nil, nil, err
	}
	domain.SortForReview(pending) // 承認しやすい順に出す（docs/spec/harness.md「まとめ承認」）
	byID = map[string]domain.PendingClaim{}
	dup := map[string]bool{}
	for _, p := range pending {
		switch {
		case p.Error != "" || p.Claim.ID == "":
			notes = append(notes, fmt.Sprintf("%s は読めない候補なので出さない（承認画面か手で確認してください）: %s", p.Name, p.Error))
		case dup[p.Claim.ID]:
		case byID[p.Claim.ID].Name != "":
			notes = append(notes, fmt.Sprintf("候補 ID %s が複数のファイルにあるので出さない", p.Claim.ID))
			dup[p.Claim.ID] = true
			delete(byID, p.Claim.ID)
		default:
			byID[p.Claim.ID] = p
			order = append(order, p.Claim.ID)
		}
	}
	kept := order[:0]
	for _, id := range order {
		if !dup[id] {
			kept = append(kept, id)
		}
	}
	return byID, kept, notes, nil
}

// Ask は承認待ち候補から、質問ダイアログに渡す質問を最大 limit 個（1〜4）作る。
// 候補が無ければ空の配列。notes は人間向けの案内（出さなかった候補・残りの件数）。
func (s *RemoteReviewService) Ask(limit int) ([]domain.AskQuestion, []string, error) {
	if limit < 1 || limit > RemoteAskMax {
		return nil, nil, fmt.Errorf("--limit は 1〜%d（質問ダイアログに 1 回で出せる数）: %d", RemoteAskMax, limit)
	}
	byID, order, notes, err := s.candidates()
	if err != nil {
		return nil, nil, err
	}
	qs := []domain.AskQuestion{}
	for _, id := range order {
		if len(qs) == limit {
			break
		}
		qs = append(qs, domain.RemoteQuestion(byID[id]))
	}
	switch {
	case len(order) == 0:
		notes = append(notes, "承認待ちの候補（claims/_pending/）はありません")
	case len(order) > len(qs):
		notes = append(notes, fmt.Sprintf("残り %d 件は、この回答を反映した後にもう一度 review ask で出してください", len(order)-len(qs)))
	}
	return qs, notes, nil
}

// Record は PostToolUse hook の stdin を読み、表示された内容が今の候補と一致する質問の回答だけを反映する。
// 失敗はすべて結果の Notes / Outcomes に入れて返す（hook はダイアログの操作を壊さないため失敗しない）。
func (s *RemoteReviewService) Record(ctx context.Context, raw []byte, by string) RemoteRecordResult {
	var res RemoteRecordResult
	var in domain.RemoteHookInput
	if err := json.Unmarshal(raw, &in); err != nil {
		res.Notes = append(res.Notes, "hook の入力（JSON）が読めない: "+err.Error())
		return res
	}
	if in.ToolName != domain.RemoteToolName || !hasRemoteQuestion(in.ToolInput.Questions) {
		// 別のツールや、spec-gate と関係ない質問ダイアログには何もしない
		return res
	}
	byID, _, _, err := s.candidates()
	if err != nil {
		res.Notes = append(res.Notes, "承認待ちの候補を読めない: "+err.Error())
		return res
	}
	current := make(map[string]domain.AskQuestion, len(byID))
	previews := make(map[string]string, len(byID))
	for id, p := range byID {
		previews[id] = domain.RenderRemotePreview(p)
		current[id] = domain.NewRemoteQuestion(id, previews[id])
	}
	decisions, notes := domain.DecideRemote(in, current)
	res.Notes = append(res.Notes, notes...)
	for _, d := range decisions {
		name := byID[d.ID].Name
		out := RemoteOutcome{Decision: d, Name: name}
		if err := s.apply(ctx, d, name, by); err != nil {
			out.Error = err.Error()
		} else {
			sum := sha256.Sum256([]byte(previews[d.ID]))
			entry := domain.ReviewDecision{Time: s.Clock.Now(), ID: d.ID, Verdict: d.Verdict, Reason: d.Reason, By: by, ContentSHA256: hex.EncodeToString(sum[:])}
			if err := s.Log.Append(entry); err != nil {
				res.Notes = append(res.Notes, fmt.Sprintf("%s: 反映したが記録に失敗: %v", d.ID, err))
			}
		}
		res.Outcomes = append(res.Outcomes, out)
	}
	return res
}

func hasRemoteQuestion(qs []domain.AskQuestion) bool {
	for _, q := range qs {
		if domain.RemoteCandidateID(q.Question) != "" {
			return true
		}
	}
	return false
}

func (s *RemoteReviewService) apply(ctx context.Context, d domain.RemoteDecision, name, by string) error {
	switch d.Verdict {
	case domain.VerdictApprove:
		_, err := s.Review.Approve(ctx, name, by)
		return err
	case domain.VerdictReject:
		return s.Review.Reject(name)
	case domain.VerdictAmbiguous:
		return s.Review.MarkAmbiguous(name, d.Reason)
	default:
		return fmt.Errorf("判定 %q は扱えない", d.Verdict)
	}
}
