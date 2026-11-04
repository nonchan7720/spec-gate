package domain

// UnverifiedRate は「unverified の件数 / claim の総数」。claim が 0 件なら 0。
// 機械判定できていない claim の割合で、CI で推移を見るための指標。
func UnverifiedRate(statuses []Status) float64 {
	if len(statuses) == 0 {
		return 0
	}
	n := 0
	for _, s := range statuses {
		if s == StatusUnverified {
			n++
		}
	}
	return float64(n) / float64(len(statuses))
}

// Statuses は判定結果の status を順に返す。
func (rs Results) Statuses() []Status {
	out := make([]Status, len(rs))
	for i, r := range rs {
		out[i] = r.Status
	}
	return out
}
