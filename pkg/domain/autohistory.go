package domain

import "time"

// AutoApprovalRecord は判定の記録から作った、自動承認 1 件の履歴。
type AutoApprovalRecord struct {
	ID        string
	Time      string
	Evidence  string
	Revoked   bool
	RevokedAt string
}

// AutoApprovals は記録（古い順）から自動承認（承認者が auto の approve）を古い順に取り出し、
// その後に同じ ID の revoke があれば差し戻し済みにする。
func AutoApprovals(ds []ReviewDecision) []AutoApprovalRecord {
	var out []AutoApprovalRecord
	open := map[string]int{} // ID → まだ差し戻されていない最新の自動承認の位置
	for _, d := range ds {
		switch {
		case d.Verdict == VerdictApprove && d.By == AutoApproverName:
			open[d.ID] = len(out)
			out = append(out, AutoApprovalRecord{ID: d.ID, Time: d.Time, Evidence: d.Evidence})
		case d.Verdict == VerdictApprove:
			// 人間が承認し直した claim の差し戻しは、前の自動承認の差し戻しではない
			delete(open, d.ID)
		case d.Verdict == VerdictRevoke:
			if i, ok := open[d.ID]; ok {
				out[i].Revoked, out[i].RevokedAt = true, d.Time
				delete(open, d.ID)
			}
		}
	}
	return out
}

// BreakerHistory はブレーカーに渡す差し戻しの並び（古い順）。resetAt（RFC 3339。時差つきでもよい）より前の差し戻しは数えない。
// 読めない時刻の差し戻しは数える（止める側に倒す）。
func BreakerHistory(recs []AutoApprovalRecord, resetAt string) []bool {
	reset, resetErr := time.Parse(time.RFC3339, resetAt)
	out := make([]bool, len(recs))
	for i, r := range recs {
		if !r.Revoked {
			continue
		}
		at, err := time.Parse(time.RFC3339, r.RevokedAt)
		out[i] = resetAt == "" || resetErr != nil || err != nil || !at.Before(reset)
	}
	return out
}
