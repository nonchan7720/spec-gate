package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/services"
)

func TestWriteCoverage_NeedsReview(t *testing.T) {
	rep := services.CoverageReport{
		Total:  1,
		ByKind: map[domain.ClaimKind]int{domain.KindExample: 1},
		Entries: []services.CoverageEntry{{
			Source:      "spec.md",
			Covered:     []string{"spec.md#端数処理"},
			Uncovered:   []string{"spec.md#休日"},
			NeedsReview: []services.NeedsReview{{Unit: "spec.md#月末処理", Reasons: []string{"M-1: 月末が休日のときが未定"}}},
		}},
	}
	var out bytes.Buffer
	uncovered := writeCoverage(&out, rep)
	if uncovered != 1 {
		t.Fatalf("要確認は uncovered に数えない（未決として記録済み）: %d", uncovered)
	}
	s := out.String()
	for _, want := range []string{"月末処理", "要確認", "M-1: 月末が休日のときが未定", "✗ 休日", "✓ 端数処理", "1 units need review"} {
		if !strings.Contains(s, want) {
			t.Errorf("%q が無い:\n%s", want, s)
		}
	}
}
