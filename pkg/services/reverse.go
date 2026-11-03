package services

import (
	"context"
	"fmt"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// ReverseService は claim を仕様文に逆変換し、元の仕様と突き合わせる（逆変換チェック）。
// 承認前のレビュー補助なので、結果は判定ではなく「人間が見るべき食い違い」の一覧。
type ReverseService struct {
	Claims   domain.ClaimRepository
	Adapters map[domain.SourceType]domain.SourceAdapter
	Reverser domain.Reverser
}

// ReverseReport は逆変換チェックの結果一覧。
type ReverseReport struct {
	Results []domain.ReverseResult
}

// Count は status ごとの件数。
func (r ReverseReport) Count(s domain.ReverseStatus) int {
	n := 0
	for _, res := range r.Results {
		if res.Status == s {
			n++
		}
	}
	return n
}

// Failed は exit 1 にすべきか。既定（strict=false）では常に false（レビュー補助なので止めない）。
// strict では mismatch と error（判定できていない）を失敗にする。skipped は失敗にしない。
func (r ReverseReport) Failed(strict bool) bool {
	return strict && (r.Count(domain.ReverseMismatch) > 0 || r.Count(domain.ReverseError) > 0)
}

// textSources は仕様テキストを持つ source。figma / image は抜粋が文章ではないので比べられない。
var textSources = map[domain.SourceType]bool{
	domain.SourceMarkdown: true,
	domain.SourceMermaid:  true,
}

// Reverse は ids の claim（空なら全 claim。_pending は ClaimRepository が返さないので対象外）を逆変換チェックする。
func (s *ReverseService) Reverse(ctx context.Context, ids []string) (ReverseReport, error) {
	all, err := s.Claims.All()
	if err != nil {
		return ReverseReport{}, err
	}
	targets := all
	if len(ids) > 0 {
		byID := map[string]domain.Claim{}
		for _, c := range all {
			byID[c.ID] = c
		}
		targets = nil
		for _, id := range ids {
			c, ok := byID[id]
			if !ok {
				return ReverseReport{}, fmt.Errorf("claim %s が見つからない（_pending の claim は対象外）", id)
			}
			targets = append(targets, c)
		}
	}

	var rep ReverseReport
	for _, c := range targets {
		rep.Results = append(rep.Results, s.reverseOne(ctx, c))
	}
	return rep, nil
}

func (s *ReverseService) reverseOne(ctx context.Context, c domain.Claim) domain.ReverseResult {
	res := domain.ReverseResult{ID: c.ID, Title: c.Title}
	if !textSources[c.Source.Type] {
		res.Status = domain.ReverseSkipped
		res.Detail = fmt.Sprintf("source type %s はテキストを持たないので比べられない", c.Source.Type)
		return res
	}
	ad, ok := s.Adapters[c.Source.Type]
	if !ok {
		res.Status = domain.ReverseSkipped
		res.Detail = fmt.Sprintf("source type %s の adapter が未登録", c.Source.Type)
		return res
	}
	snap, err := ad.Snapshot(ctx, c.Source.Ref)
	if err != nil {
		res.Status = domain.ReverseError
		res.Detail = fmt.Sprintf("仕様を読めない: %v", err)
		return res
	}
	res.Spec = snap.Location
	if snap.Skipped != "" {
		res.Status = domain.ReverseSkipped
		res.Detail = "仕様を読めない: " + snap.Skipped
		return res
	}
	if snap.Excerpt == "" {
		res.Status = domain.ReverseSkipped
		res.Detail = "仕様の抜粋が空"
		return res
	}

	// 再生成には元の仕様を渡さない。claim だけから書き起こした文が仕様と一致するかを見たいので
	restated, err := s.Reverser.Restate(ctx, c)
	if err != nil {
		res.Status = domain.ReverseError
		res.Detail = fmt.Sprintf("仕様文の再生成に失敗: %v", err)
		return res
	}
	res.Restated = restated

	cmp, err := s.Reverser.Compare(ctx, restated, snap.Excerpt)
	if err != nil {
		// 解釈できない応答を一致とみなすと、確認していないのに通ってしまう
		res.Status = domain.ReverseError
		res.Detail = fmt.Sprintf("比較に失敗: %v", err)
		return res
	}
	res.Differences = cmp.Differences
	if cmp.Match {
		res.Status = domain.ReverseMatch
	} else {
		res.Status = domain.ReverseMismatch
	}
	return res
}
