package domain

import (
	"sort"
	"strconv"
	"strings"
)

// まとめ承認と候補の並び順（docs/spec/harness.md「まとめ承認」）。

// BulkAskMax は review ask --bulk で 1 つの質問にまとめる候補の上限。
const BulkAskMax = 8

// ParseConfidence は `n/N` を読む。読めなければ ok=false。
func ParseConfidence(s string) (n, total int, ok bool) {
	a, b, found := strings.Cut(strings.TrimSpace(s), "/")
	if !found {
		return 0, 0, false
	}
	n, err1 := strconv.Atoi(a)
	total, err2 := strconv.Atoi(b)
	if err1 != nil || err2 != nil || total <= 0 || n < 0 || n > total {
		return 0, 0, false
	}
	return n, total, true
}

var dryRunRank = map[DryRunState]int{DryRunPass: 0, DryRunNone: 1, DryRunFail: 2, DryRunError: 3}

var kindRank = map[ClaimKind]int{KindExample: 0, KindStructure: 1, KindUI: 2, KindManual: 3}

func rankOf(m map[ClaimKind]int, k ClaimKind) int {
	if r, ok := m[k]; ok {
		return r
	}
	return len(m)
}

// ReviewLess は承認しやすい順の比較。試し実行（pass → none → fail → error）、合意数の割合の大きい順、
// kind（example → structure → ui → manual）、ファイル名の順。
func ReviewLess(a, b PendingClaim) bool {
	if ra, rb := dryRunRank[DryRunStateOf(a.DryRun)], dryRunRank[DryRunStateOf(b.DryRun)]; ra != rb {
		return ra < rb
	}
	fa, fb := agreeRatio(a.Confidence), agreeRatio(b.Confidence)
	if fa != fb {
		return fa > fb
	}
	if ka, kb := rankOf(kindRank, a.Claim.Kind), rankOf(kindRank, b.Claim.Kind); ka != kb {
		return ka < kb
	}
	return a.Name < b.Name
}

func agreeRatio(conf string) float64 {
	n, total, ok := ParseConfidence(conf)
	if !ok {
		return 0
	}
	return float64(n) / float64(total)
}

// SortForReview は候補を承認しやすい順に並べ替える。
func SortForReview(ps []PendingClaim) {
	sort.SliceStable(ps, func(i, j int) bool { return ReviewLess(ps[i], ps[j]) })
}

// BulkEligible はまとめ承認の対象か。合意数が minAgree 以上（0 なら全員一致）で、kind が example なら試し実行が pass。
// 読めない候補は対象外。dry-run はヘッダの値なので、承認するときにもう一度試し実行して確かめる。
func BulkEligible(p PendingClaim, minAgree int) bool {
	if p.Error != "" {
		return false
	}
	n, total, ok := ParseConfidence(p.Confidence)
	if !ok {
		return false
	}
	need := minAgree
	if need <= 0 {
		need = total
	}
	if n < need {
		return false
	}
	if p.Claim.NeedsDryRun() && DryRunStateOf(p.DryRun) != DryRunPass {
		return false
	}
	return true
}

// ManualRate は manual の claim の数 / claim の総数（0 件なら 0）。docs/spec/harness.md「manual の手がかり」。
func ManualRate(kinds []ClaimKind) float64 {
	if len(kinds) == 0 {
		return 0
	}
	n := 0
	for _, k := range kinds {
		if k == KindManual {
			n++
		}
	}
	return float64(n) / float64(len(kinds))
}
