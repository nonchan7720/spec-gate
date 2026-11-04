package domain

// SuggestHeading は、見つからない見出し missing に最も近い見出しを headings（文書の順）から選ぶ。
// 近さは文字（rune）単位の編集距離で測り、距離が missing の文字数の半分以下のものだけを候補にする。
// 同じ距離なら文書の順で先のものを選ぶ。候補が無ければ ok は false。
// 提案だけを返し、claim は書き換えない（直すのは人間）。
func SuggestHeading(missing string, headings []string) (heading string, ok bool) {
	n := len([]rune(missing))
	best := -1
	for _, h := range headings {
		d := editDistance(missing, h)
		// 「半分以下」を整数の切り捨てなしで比べる: d <= n/2 ⇔ 2d <= n
		if 2*d > n {
			continue
		}
		if best < 0 || d < best { // 同点は先に出たものを残す
			best, heading, ok = d, h, true
		}
	}
	return heading, ok
}

// editDistance は a と b の文字（rune）単位のレーベンシュタイン距離。
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}
