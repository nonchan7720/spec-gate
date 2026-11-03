package services

import (
	"context"
	"errors"
	"fmt"
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

// List は承認待ち候補を、仕様の現在の抜粋つきで返す。
func (s *ReviewService) List(ctx context.Context) ([]ReviewItem, error) {
	s.mu.Lock()
	pending, err := s.Pending.Pending()
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
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
