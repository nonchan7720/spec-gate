package tracelog

import (
	"path/filepath"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// DecisionsRelPath はリモート承認で反映した判定の記録ファイル（プロジェクトルートから）。
const DecisionsRelPath = ".spec-gate/review-decisions.jsonl"

// Decisions はリモート承認の判定を 1 件 1 行で追記する記録。
type Decisions struct{ path string }

var _ domain.ReviewDecisionLog = (*Decisions)(nil)

func NewDecisions(root string) *Decisions {
	return &Decisions{path: filepath.Join(root, DecisionsRelPath)}
}

// Append は判定 1 件を追記する。
func (l *Decisions) Append(d domain.ReviewDecision) error {
	return appendJSONL(l.path, []domain.ReviewDecision{d})
}
