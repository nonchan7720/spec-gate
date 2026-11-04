package domain

import (
	"strconv"
	"strings"
)

// Mutant はソースに入れる変異 1 つ。1 回に 1 か所だけ変える。
type Mutant struct {
	Line   int    // 1 始まりの行番号
	Before string // 変異前の行
	After  string // 変異後の行
}

// Apply は src の Line 行目を After に置き換えたソースを返す。
func (m Mutant) Apply(src string) string {
	lines := strings.Split(src, "\n")
	if m.Line < 1 || m.Line > len(lines) {
		return src
	}
	lines[m.Line-1] = m.After
	return strings.Join(lines, "\n")
}

// FileMutant はファイルの位置つきの変異。
type FileMutant struct {
	File string
	Mutant
}

// MutationResult は mutation score の集計。
type MutationResult struct {
	// Candidates は作れた変異の総数（--max で間引く前）。
	Candidates int
	// Total は実際に verify を走らせた変異の数。
	Total int
	// Killed は verify のどれかが落ちた（＝変異を検出できた）数。
	Killed int
	// Survivors は verify がすべて通ってしまった変異。
	Survivors []FileMutant
}

// Score は Killed / Total。変異が 0 件なら 0（検証の強さを示す根拠が無いので低い側に倒す）。
func (r MutationResult) Score() float64 {
	if r.Total == 0 {
		return 0
	}
	return float64(r.Killed) / float64(r.Total)
}

// PickEvenly は xs から max 件を均等な間隔で選ぶ。max <= 0 か件数以下なら全部返す。
// 先頭のファイルばかりに偏らないようにするため。
func PickEvenly[T any](xs []T, max int) []T {
	if max <= 0 || len(xs) <= max {
		return xs
	}
	out := make([]T, 0, max)
	for i := range max {
		out = append(out, xs[i*len(xs)/max])
	}
	return out
}

// mutationOps は演算子ごとの変異先（この順で出す）。仕様の変異一覧がすべてで、ここに無い演算子は変えない。
var mutationOps = map[string][]string{
	// 算術
	"+": {"-"},
	"-": {"+"},
	"*": {"/"},
	"/": {"*"},
	// 比較
	">":  {">=", "<"},
	"<":  {"<=", ">"},
	">=": {">"},
	"<=": {"<"},
	"==": {"!="},
	"!=": {"=="},
}

// minMutatedLiteral は変異させる整数リテラルの下限。0 と 1 は添字や真偽に使われやすいので変えない。
const minMutatedLiteral = 2

// MutateLine はコード 1 行に 1 か所だけ変異を入れた行を、行の左から順に返す。
func MutateLine(line string) []string {
	out := []string{}
	for _, m := range MutateSource(line) {
		out = append(out, m.After)
	}
	return out
}

// site は変異を入れられる 1 か所（src のバイト範囲と置き換え候補）。
type site struct {
	start, end int
	repl       []string
}

// MutateSource はソース全体を走査し、文字列リテラルとコメントを飛ばして変異を列挙する。
// 複数行にまたがるコメント（/* */）や raw string（`...`）の中も変えない。
func MutateSource(src string) []Mutant {
	var sites []site
	n := len(src)
	for i := 0; i < n; {
		c := src[i]
		switch {
		case c == '/' && i+1 < n && src[i+1] == '/':
			i = skipUntil(src, i+2, "\n")
		case c == '/' && i+1 < n && src[i+1] == '*':
			end := strings.Index(src[i+2:], "*/")
			if end < 0 {
				i = n
			} else {
				i = i + 2 + end + 2
			}
		case c == '"' || c == '\'':
			i = skipQuoted(src, i+1, c)
		case c == '`':
			end := strings.IndexByte(src[i+1:], '`')
			if end < 0 {
				i = n
			} else {
				i = i + 1 + end + 1
			}
		case isDigit(c):
			j := i
			for j < n && (isIdentByte(src[j]) || src[j] == '.') {
				j++
			}
			if s, ok := literalSite(src, i, j); ok {
				sites = append(sites, s)
			}
			i = j
		case isIdentByte(c):
			for i < n && isIdentByte(src[i]) {
				i++
			}
		case isOpByte(c):
			j := i
			// 演算子の並びはコメント開始（// や /*）の手前で切る
			for j < n && isOpByte(src[j]) && !isCommentStart(src, j) {
				j++
			}
			if repl, ok := mutationOps[src[i:j]]; ok && isBinary(src, i, j) {
				sites = append(sites, site{start: i, end: j, repl: repl})
			}
			i = j
		default:
			i++
		}
	}

	var out []Mutant
	for _, s := range sites {
		ls := strings.LastIndexByte(src[:s.start], '\n') + 1
		le := strings.IndexByte(src[s.end:], '\n')
		if le < 0 {
			le = n
		} else {
			le += s.end
		}
		line := strings.Count(src[:s.start], "\n") + 1
		before := src[ls:le]
		for _, r := range s.repl {
			out = append(out, Mutant{
				Line:   line,
				Before: before,
				After:  src[ls:s.start] + r + src[s.end:le],
			})
		}
	}
	return out
}

// literalSite は src[i:j] が 2 以上の 10 進整数リテラルなら +1 する変異を返す。
// 識別子の一部（v2）・小数（1.5）・指数（1e9）・16 進（0x10）・区切り（1_000）・先頭 0（0755）は対象外。
func literalSite(src string, i, j int) (site, bool) {
	if i > 0 && (isIdentByte(src[i-1]) || src[i-1] == '.') {
		return site{}, false
	}
	tok := src[i:j]
	for k := 0; k < len(tok); k++ {
		if !isDigit(tok[k]) {
			return site{}, false
		}
	}
	if tok[0] == '0' {
		return site{}, false
	}
	v, err := strconv.ParseUint(tok, 10, 64)
	if err != nil || v < minMutatedLiteral || v == ^uint64(0) {
		return site{}, false
	}
	return site{start: i, end: j, repl: []string{strconv.FormatUint(v+1, 10)}}, true
}

// isBinary は演算子 src[i:j] が二項演算子として書かれているか。
// 前後が空白で、同じ行の前に何かがあることを条件にする（*p・-x・[]*T のような単項やポインタ型を変えない）。
func isBinary(src string, i, j int) bool {
	if i == 0 || !isSpace(src[i-1]) {
		return false
	}
	if j < len(src) && !isSpace(src[j]) && src[j] != '\n' && src[j] != '\r' {
		return false
	}
	ls := strings.LastIndexByte(src[:i], '\n') + 1
	return strings.TrimSpace(src[ls:i]) != ""
}

func skipUntil(src string, i int, sep string) int {
	end := strings.Index(src[i:], sep)
	if end < 0 {
		return len(src)
	}
	return i + end
}

// skipQuoted は q で閉じる文字列リテラル（エスケープあり、改行で打ち切り）の直後を返す。
func skipQuoted(src string, i int, q byte) int {
	for i < len(src) {
		switch src[i] {
		case '\\':
			i += 2
		case q:
			return i + 1
		case '\n':
			return i
		default:
			i++
		}
	}
	return len(src)
}

// isCommentStart は src[i:] が // か /* で始まるか。
func isCommentStart(src string, i int) bool {
	return src[i] == '/' && i+1 < len(src) && (src[i+1] == '/' || src[i+1] == '*')
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isIdentByte(c byte) bool {
	return c == '_' || isDigit(c) || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

func isOpByte(c byte) bool { return strings.IndexByte("+-*/<>=!&|^%:?", c) >= 0 }

func isSpace(c byte) bool { return c == ' ' || c == '\t' }
