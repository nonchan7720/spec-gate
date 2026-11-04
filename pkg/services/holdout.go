package services

import (
	"errors"
	"fmt"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// HoldoutService は holdout を足す（spec-gate holdout add）。人間だけが使うコマンドで、
// エージェントからの実行は hook-gate の pre_bash で拒否する（docs/spec/harness.md「holdout examples」）。
// 期待値は正規化 JSON の SHA-256 だけを書き、平文はどこにも保存しない。
type HoldoutService struct {
	Claims domain.ClaimRepository
	Writer domain.HoldoutWriter
}

// Add は claim の holdout に in と out のハッシュを 1 件足し、書いたファイルのパスを返す。
// claim が無い・kind: example でない claim には足さない（lint で使われない holdout になる）。
// エラーに in / out の値は含めない。
func (s *HoldoutService) Add(claimID string, in, out any) (string, error) {
	claims, err := s.Claims.All()
	if err != nil {
		return "", err
	}
	var found *domain.Claim
	for i := range claims {
		if claims[i].ID == claimID {
			found = &claims[i]
			break
		}
	}
	switch {
	case found == nil:
		return "", fmt.Errorf("holdout: claim %s が無い（承認済みの claim にだけ足せる）", claimID)
	case found.Kind != domain.KindExample:
		return "", fmt.Errorf("holdout: claim %s は kind: %s。holdout は kind: example の claim にだけ使える", claimID, found.Kind)
	}
	h, err := domain.OutputHash(out)
	if err != nil {
		return "", errors.New("holdout: out を JSON にできない（値は表示しない）")
	}
	path, err := s.Writer.Append(claimID, domain.HoldoutExample{In: in, OutSHA256: h})
	if err != nil {
		return "", fmt.Errorf("holdout: %s に書けない: %w", claimID, err)
	}
	return path, nil
}
