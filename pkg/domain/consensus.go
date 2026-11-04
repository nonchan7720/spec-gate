package domain

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// ParaphraseThreshold は、例の無い claim の title を言い換えとみなす類似度（文字 bigram の Dice 係数）の下限。
// 根拠は docs/spec/harness.md の「合議の同定」。
const ParaphraseThreshold = 0.6

// Agreement は合議で 1 つにまとめた主張と、それを出した run の数。
type Agreement struct {
	Claim Claim
	Hits  int
	// Members は束に入った claim（run の順。1 run 1 件）。束ねた理由を人間が確かめるために残す
	Members []Claim
}

// MergeRuns は run ごとに独立に抽出した claim を「同じ主張」ごとに束ね、初出順に返す。
//   - 例のある claim は kind と (in, out) の組の集合が一致すれば同じ主張
//   - 例の無い claim は、同じ ref・同じ kind の例の無い束のうち、正規化した title の類似度
//     （TitleSimilarity）が ParaphraseThreshold 以上のものに入る。束の類似度はその束の title との最大値
//   - 1 つの run は 1 つの束に 1 件まで。候補が競合したら、類似度の高い組から順に決める
//     （同点は run 内で先に出た claim、先にできた束を優先）。どこにも入らない claim は新しい束になる
//   - 束の代表は例の多い claim（同数なら先に出たもの）
func MergeRuns(perRun [][]Claim) []Agreement {
	type bucket struct {
		claim   Claim
		hits    int
		members []Claim
		titles  []string // 例の無い束の、正規化した title（run の順）
		runs    map[int]bool
		all     []string // 束に入ったすべての claim の正規化した title（kind 跨ぎの照合用）
	}
	add := func(b *bucket, run int, c Claim) {
		if b.runs == nil {
			b.runs = map[int]bool{}
		}
		b.runs[run] = true
		b.all = append(b.all, NormalizeClaimTitle(c.Title))
	}
	var buckets []*bucket
	byKey := map[string]*bucket{}

	for run, claims := range perRun {
		taken := map[*bucket]bool{}
		seen := map[string]bool{}
		var loose []Claim // 例の無い claim（言い換えで照合する）
		for _, c := range claims {
			key := ClaimFingerprint(c)
			if seen[key] {
				continue
			}
			seen[key] = true
			if len(c.Examples) == 0 {
				loose = append(loose, c)
				continue
			}
			if b, ok := byKey[key]; ok {
				b.hits++
				b.members = append(b.members, c)
				add(b, run, c)
				taken[b] = true
				if len(c.Examples) > len(b.claim.Examples) { // より情報量の多い方を残す
					b.claim = c
				}
				continue
			}
			b := &bucket{claim: c, hits: 1, members: []Claim{c}}
			add(b, run, c)
			byKey[key] = b
			buckets = append(buckets, b)
			taken[b] = true
		}

		type match struct {
			ci, bi int
			sim    float64
		}
		var ms []match
		norms := make([]string, len(loose))
		for ci, c := range loose {
			norms[ci] = NormalizeClaimTitle(c.Title)
			for bi, b := range buckets {
				if b.titles == nil || b.claim.Kind != c.Kind || b.claim.Source.Ref != c.Source.Ref {
					continue
				}
				best := 0.0
				for _, t := range b.titles {
					best = max(best, TitleSimilarity(norms[ci], t))
				}
				if best >= ParaphraseThreshold {
					ms = append(ms, match{ci, bi, best})
				}
			}
		}
		sort.SliceStable(ms, func(i, j int) bool {
			if ms[i].sim != ms[j].sim {
				return ms[i].sim > ms[j].sim
			}
			if ms[i].ci != ms[j].ci {
				return ms[i].ci < ms[j].ci
			}
			return ms[i].bi < ms[j].bi
		})
		placed := make([]bool, len(loose))
		for _, m := range ms {
			b := buckets[m.bi]
			if placed[m.ci] || taken[b] {
				continue
			}
			placed[m.ci], taken[b] = true, true
			b.hits++
			b.members = append(b.members, loose[m.ci])
			b.titles = append(b.titles, norms[m.ci])
			add(b, run, loose[m.ci])
		}
		for ci, c := range loose {
			if placed[ci] {
				continue
			}
			b := &bucket{claim: c, hits: 1, members: []Claim{c}, titles: []string{norms[ci]}}
			add(b, run, c)
			buckets = append(buckets, b)
			taken[b] = true
		}
	}

	// manual の束を、同じ主張の example の束に寄せる（docs/spec/harness.md「manual と例のある claim の同定」）
	merged := map[*bucket]bool{}
	for _, m := range buckets {
		if m.claim.Kind != KindManual {
			continue
		}
		var best *bucket
		bestSim := 0.0
		for _, e := range buckets {
			if e.claim.Kind != KindExample || merged[e] || e.claim.Source.Ref != m.claim.Source.Ref || overlaps(e.runs, m.runs) {
				continue
			}
			sim := 0.0
			for _, a := range m.all {
				for _, b := range e.all {
					sim = max(sim, TitleSimilarity(a, b))
				}
			}
			if sim >= ParaphraseThreshold && sim > bestSim {
				best, bestSim = e, sim
			}
		}
		if best == nil {
			continue
		}
		best.hits += m.hits
		best.members = append(best.members, m.members...)
		best.all = append(best.all, m.all...)
		for r := range m.runs {
			best.runs[r] = true
		}
		merged[m] = true
	}

	out := make([]Agreement, 0, len(buckets))
	for _, b := range buckets {
		if merged[b] {
			continue
		}
		c := b.claim
		if c.Hint == "" {
			for _, mem := range b.members {
				if mem.Hint != "" {
					c.Hint = mem.Hint
					break
				}
			}
		}
		out = append(out, Agreement{Claim: c, Hits: b.hits, Members: b.members})
	}
	return out
}

func overlaps(a, b map[int]bool) bool {
	for k := range a {
		if b[k] {
			return true
		}
	}
	return false
}

// ClaimFingerprint は 1 つの run の中での重複と、例のある claim の同定に使うキー。
// 例があれば kind + (in, out) の組の集合（title の言い回しは実行ごとに揺れるので見ない）。
// 例が無ければ kind + ref + 正規化した title。
func ClaimFingerprint(c Claim) string {
	if len(c.Examples) > 0 {
		set := map[string]bool{}
		for _, e := range c.Examples {
			set[fmt.Sprintf("%#v=>%#v", e.In, e.Out)] = true
		}
		pairs := make([]string, 0, len(set))
		for p := range set {
			pairs = append(pairs, p)
		}
		sort.Strings(pairs)
		return string(c.Kind) + "|ex|" + strings.Join(pairs, "\x00")
	}
	return string(c.Kind) + "|title|" + c.Source.Ref + "|" + NormalizeClaimTitle(c.Title)
}

var (
	// 【要確認: …】 の注記。閉じ括弧が無ければ末尾まで。
	reviewNote = regexp.MustCompile(`【\s*要確認[^】]*(】|$)`)
	// [M-1] / (HOLDOUT-003) / 【R-2】 のような括弧つきの ID。
	bracketedID = regexp.MustCompile(`[\[(（【]\s*[A-Za-z][A-Za-z0-9_]*(-[A-Za-z0-9_]+)+\s*[\])）】]`)
)

// titleOperators は記号でも落とさない文字。演算子の並び（算術と比較の変異など）が主張の中身になるため。
const titleOperators = "+-*/<>=!"

// NormalizeClaimTitle は title から【要確認…】の注記（と「要確認」以降）・括弧つき ID を除き、
// 演算子（+ - * / < > = !）以外の句読点・記号・空白（バッククォート・括弧の文字も含む）を落として小文字にする。
// 括弧の中身は残す（主張を区別する内容が括弧書きにあることが多い）。
func NormalizeClaimTitle(t string) string {
	t = reviewNote.ReplaceAllString(t, "")
	t = strings.Split(t, "要確認")[0]
	t = bracketedID.ReplaceAllString(t, "")
	var b strings.Builder
	for _, r := range t {
		if strings.ContainsRune(titleOperators, r) {
			b.WriteRune(r)
			continue
		}
		if unicode.IsPunct(r) || unicode.IsSymbol(r) || unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// TitleSimilarity は正規化済みの 2 つの title の、文字 bigram の集合の Dice 係数（0〜1）。
// 同じ文字列なら 1。bigram が作れない（1 文字以下の）異なる文字列は 0。
func TitleSimilarity(a, b string) float64 {
	if a == b {
		return 1
	}
	x, y := bigrams(a), bigrams(b)
	if len(x)+len(y) == 0 {
		return 0
	}
	common := 0
	for g := range x {
		if y[g] {
			common++
		}
	}
	return 2 * float64(common) / float64(len(x)+len(y))
}

func bigrams(s string) map[string]bool {
	rs := []rune(s)
	out := map[string]bool{}
	for i := 0; i+1 < len(rs); i++ {
		out[string(rs[i:i+2])] = true
	}
	return out
}
