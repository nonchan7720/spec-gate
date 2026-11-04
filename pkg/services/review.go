package services

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// ReviewService は承認待ち候補（claims/_pending/）を人間が確認して承認・却下するための操作。
// 承認は「claims/<領域>/ へ移す → ApproveService で lint と仕様のハッシュ記録」。
// ApproveService が拒否したら _pending に戻し、承認されないまま領域に残る claim を作らない。
type ReviewService struct {
	Pending  domain.PendingRepository
	Adapters map[domain.SourceType]domain.SourceAdapter
	Approver *ApproveService

	// mu はファイル移動と承認を直列化する（画面から同時に押されても途中状態を他の操作に見せない）。
	mu sync.Mutex
}

// ReviewItem は画面に出す候補 1 件。
type ReviewItem struct {
	domain.PendingClaim
	// Area は承認時の移動先（claims/<Area>/）。壊れた候補は空
	Area string
	// Spec は仕様の現在の抜粋
	Spec domain.Snapshot
	// SpecError は仕様を読めなかった理由
	SpecError string
}

// AreaFor は claim の種類から移動先の領域を決める。
func AreaFor(k domain.ClaimKind) string {
	switch k {
	case domain.KindStructure:
		return "arch"
	case domain.KindUI:
		return "ui"
	default:
		return "spec"
	}
}

// List は承認待ち候補を、承認しやすい順（domain.SortForReview）に、仕様の現在の抜粋つきで返す。
func (s *ReviewService) List(ctx context.Context) ([]ReviewItem, error) {
	s.mu.Lock()
	pending, err := s.Pending.Pending()
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	domain.SortForReview(pending)
	items := make([]ReviewItem, 0, len(pending))
	for _, p := range pending {
		it := ReviewItem{PendingClaim: p}
		if p.Error == "" {
			it.Area = AreaFor(p.Claim.Kind)
			it.Spec, it.SpecError = s.snapshot(ctx, p.Claim.Source)
		}
		items = append(items, it)
	}
	return items, nil
}

func (s *ReviewService) snapshot(ctx context.Context, src domain.Source) (domain.Snapshot, string) {
	ad, ok := s.Adapters[src.Type]
	if !ok {
		return domain.Snapshot{}, fmt.Sprintf("source type %s の adapter が未登録", src.Type)
	}
	snap, err := ad.Snapshot(ctx, src.Ref)
	if err != nil {
		return domain.Snapshot{}, err.Error()
	}
	if snap.Skipped != "" {
		return snap, "仕様を読めない: " + snap.Skipped
	}
	return snap, ""
}

// Approve は候補 name を claims/<領域>/ に移して承認する。拒否されたら _pending に戻す。
func (s *ReviewService) Approve(ctx context.Context, name, by string) (ApproveResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	target, err := s.find(name)
	if err != nil {
		return ApproveResult{}, err
	}
	if target.Error != "" {
		return ApproveResult{}, fmt.Errorf("候補 %s を読めないので承認できない: %s", name, target.Error)
	}
	path, err := s.Pending.Promote(name, AreaFor(target.Claim.Kind))
	if err != nil {
		return ApproveResult{}, err
	}
	res, err := s.Approver.Approve(ctx, target.Claim.ID, by)
	if err != nil {
		if derr := s.Pending.Demote(path); derr != nil {
			return ApproveResult{}, errors.Join(err, fmt.Errorf("_pending に戻せなかった（%s を確認してください）: %w", path, derr))
		}
		return ApproveResult{}, err
	}
	return res, nil
}

// BulkOutcome はまとめ承認の 1 件分の結果。Error が空なら承認できた。
type BulkOutcome struct {
	Name  string `json:"name"`
	ID    string `json:"id,omitempty"`
	Error string `json:"error,omitempty"`
}

// BulkTargets はまとめ承認の対象（domain.BulkEligible）を承認しやすい順に返す。kinds が空でなければ kind で絞る。
// ID が重複する候補は対象にしない。dry-run はヘッダの値なので、承認時に BulkApprove が試し実行し直す。
func (s *ReviewService) BulkTargets(minAgree int, kinds []domain.ClaimKind) ([]domain.PendingClaim, error) {
	s.mu.Lock()
	pending, err := s.Pending.Pending()
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	ids := map[string]int{}
	for _, p := range pending {
		ids[p.Claim.ID]++
	}
	var out []domain.PendingClaim
	for _, p := range pending {
		if ids[p.Claim.ID] > 1 || !domain.BulkEligible(p, minAgree) {
			continue
		}
		if len(kinds) > 0 && !slices.Contains(kinds, p.Claim.Kind) {
			continue
		}
		out = append(out, p)
	}
	domain.SortForReview(out)
	return out, nil
}

// BulkApprove は候補を 1 件ずつ Approve と同じ処理で承認する。1 件が拒否されても残りは続ける。
// requirePass なら、binding のある example 候補は承認の前に試し実行し、pass でなければ承認しない（ヘッダは信用しない）。
func (s *ReviewService) BulkApprove(ctx context.Context, names []string, by string, requirePass bool) []BulkOutcome {
	out := make([]BulkOutcome, 0, len(names))
	for _, name := range names {
		o := BulkOutcome{Name: name}
		if requirePass {
			s.mu.Lock()
			target, err := s.find(name)
			s.mu.Unlock()
			if err != nil {
				o.Error = err.Error()
				out = append(out, o)
				continue
			}
			o.ID = target.Claim.ID
			if s.Approver.Probe != nil && target.Claim.NeedsDryRun() {
				if d := s.Approver.Probe.Probe(ctx, target.Claim); d.State() != domain.DryRunPass {
					o.Error = "試し実行が pass でないのでまとめ承認しない（1 件ずつ確かめて承認する）: " + d.Summary()
					out = append(out, o)
					continue
				}
			}
		}
		res, err := s.Approve(ctx, name, by)
		if err != nil {
			o.Error = err.Error()
		} else {
			o.ID = res.Claim.ID
		}
		out = append(out, o)
	}
	return out
}

// Probe は承認待ち候補をすべて試し実行し直し、`# dry-run:` を書き換える（review probe）。
// 返り値は候補名 → 書いた結果。試し実行の対象でない候補は含めない。
func (s *ReviewService) Probe(ctx context.Context) (map[string]string, error) {
	if s.Approver == nil || s.Approver.Probe == nil {
		return nil, errors.New("試し実行の仕組みが組み込まれていない")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, err := s.Pending.Pending()
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, p := range pending {
		if p.Error != "" || !p.Claim.NeedsDryRun() {
			continue
		}
		sum := s.Approver.Probe.Probe(ctx, p.Claim).Summary()
		if err := s.Pending.WriteDryRun(p.Name, sum); err != nil {
			return out, err
		}
		out[p.Name] = sum
	}
	return out, nil
}

// Reject は候補 name を削除する。
func (s *ReviewService) Reject(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.find(name); err != nil {
		return err
	}
	return s.Pending.DiscardPending(name)
}

// MarkAmbiguous は候補 name を「仕様が決まっていない」として claims/_ambiguous/ に記録する。
// claim の誤り（却下）ではなく仕様側の未決を残すためのもので、理由は必須。
// 読めない候補も記録できる（仕様が決まらないために claim を書けなかった、もありうる）。
func (s *ReviewService) MarkAmbiguous(name, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return errors.New("曖昧とする理由を入力してください")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.find(name); err != nil {
		return err
	}
	_, err := s.Pending.MarkAmbiguous(name, reason)
	return err
}

func (s *ReviewService) find(name string) (domain.PendingClaim, error) {
	pending, err := s.Pending.Pending()
	if err != nil {
		return domain.PendingClaim{}, err
	}
	for _, p := range pending {
		if p.Name == name {
			return p, nil
		}
	}
	return domain.PendingClaim{}, fmt.Errorf("候補 %s がない", name)
}
