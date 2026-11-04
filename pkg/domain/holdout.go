package domain

import (
	"fmt"
	"sort"
)

// holdout はエージェントに見せない入出力例（holdout/<claim-id>.yaml）。
// check では examples に続けて binding に流すが、レポートやエージェントへの出力には
// 入力・期待値・実際の値を一切出さず、件数だけを出す。

// HoldoutResult は holdout 1 件の判定結果。値は持たない（持たせると出力に混ざる危険がある）。
type HoldoutResult struct {
	Pass bool `json:"pass"`
}

// HoldoutSummary はレポートに出す 1 行。件数だけを出し、値は出さない。
func HoldoutSummary(results []HoldoutResult) string {
	if len(results) == 0 {
		return "holdout なし"
	}
	failed := 0
	for _, r := range results {
		if !r.Pass {
			failed++
		}
	}
	if failed == 0 {
		return fmt.Sprintf("holdout %d 件すべて一致", len(results))
	}
	return fmt.Sprintf("holdout %d 件中 %d 件が不一致（値は表示しない）", len(results), failed)
}

// LintHoldout は holdout が properties を満たすかを確かめる（examples の Lint と同じ観点）。
// メッセージには holdout の番号だけを出し、値や評価エラーの本文（値や型が混ざる）は出さない。
// holdout は claim ID → examples。
func LintHoldout(claims []Claim, holdout map[string][]Example) []LintIssue {
	byID := map[string]Claim{}
	for _, c := range claims {
		byID[c.ID] = c
	}
	ids := make([]string, 0, len(holdout))
	for id := range holdout {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var issues []LintIssue
	for _, id := range ids {
		exs := holdout[id]
		c, ok := byID[id]
		switch {
		case !ok:
			issues = append(issues, LintIssue{id, "holdout に対応する claim が無い"})
			continue
		case c.Kind != KindExample:
			issues = append(issues, LintIssue{id, fmt.Sprintf("holdout は kind: example の claim にだけ使える（kind: %s）", c.Kind)})
			continue
		}
		for _, p := range c.Properties {
			for i, ex := range exs {
				ok, err := EvalProperty(p, ex.In, ex.Out)
				if err != nil {
					issues = append(issues, LintIssue{id, fmt.Sprintf("holdout[%d] で property %q を評価できない（値は表示しない）", i, p)})
					continue
				}
				if !ok {
					issues = append(issues, LintIssue{id, fmt.Sprintf("holdout[%d] が property %q を満たさない（値は表示しない）", i, p)})
				}
			}
		}
	}
	return issues
}
