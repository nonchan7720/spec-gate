package domain

// TraceEntry は check --hook 1 回分の、claim 1 件の判定記録（.spec-gate/trace.jsonl の 1 行）。
type TraceEntry struct {
	Time   string `json:"time"`
	ID     string `json:"id"`
	Status Status `json:"status"`
	// Lines はその時点の変更行数（git diff --numstat HEAD の追加行と削除行の合計。git でなければ 0）
	Lines int `json:"lines"`
}

// TraceSummary は claim が最後に pass になったときの通り方。
type TraceSummary struct {
	// FailsBeforePass はその pass の直前の連続 fail 回数
	FailsBeforePass int `json:"fails_before_pass"`
	// LinesAtPass はその pass の時点の変更行数
	LinesAtPass int `json:"lines_at_pass"`
}

// SummarizeTrace は 1 つの claim の判定履歴（古い順）から、最後に pass になったとき
// （pass でない判定の次の pass、または最初の判定が pass）について、直前の連続 fail 回数と変更行数を返す。
// pass が無ければ nil。連続 fail は fail 以外（stale など）で途切れる。
func SummarizeTrace(history []TraceEntry) *TraceSummary {
	at := -1
	for i, e := range history {
		if e.Status == StatusPass && (i == 0 || history[i-1].Status != StatusPass) {
			at = i
		}
	}
	if at < 0 {
		return nil
	}
	fails := 0
	for i := at - 1; i >= 0 && history[i].Status == StatusFail; i-- {
		fails++
	}
	return &TraceSummary{FailsBeforePass: fails, LinesAtPass: history[at].Lines}
}
