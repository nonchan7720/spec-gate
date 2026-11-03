package domain

import (
	"encoding/json"
	"path"
	"sort"
	"strings"
	"unicode/utf8"
)

// リテラル漏れ検知: 実装に claim の examples・holdout の値がそのまま書き込まれていないかを調べる
// （docs/spec/harness.md「リテラル漏れ検知」）。例の値を直書きして通す実装を見つけるため。

const (
	// leakMinDigits は対象にする数値の桁数の下限。桁は符号と小数点を除いた 0〜9 の個数
	leakMinDigits = 3
	// leakMinRunes は対象にする文字列の文字数の下限
	leakMinRunes = 4
)

// Leak は見つかった値 1 件（値・探した表記・行番号）。同じ値が同じ行に複数あっても 1 件。
type Leak struct {
	Value any
	Text  string
	Line  int
}

// FindLeaks は code に values の値が現れる位置を、値の順・行の順で返す。
// 配列・オブジェクトの値は中の値を調べる。対象外の値（短い数値・短い文字列・真偽値・null）は無視する。
func FindLeaks(code string, values []any) []Leak {
	var leaks []Leak
	seen := map[string]bool{}
	for _, v := range flatten(values) {
		text, isNum, ok := leakText(v)
		if !ok || seen[text] {
			continue
		}
		seen[text] = true
		last := 0
		for _, off := range occurrences(code, text, isNum) {
			line := 1 + strings.Count(code[:off], "\n")
			if line == last {
				continue
			}
			last = line
			// YAML の int も float64 に揃える（JSON にすれば同じ表記）
			leaks = append(leaks, Leak{Value: normalizeNumber(v), Text: text, Line: line})
		}
	}
	return leaks
}

// LeakedValues は code に現れた値を、values の順に重複なしで返す（binding `leak` の出力）。
func LeakedValues(code string, values []any) []any {
	out := []any{}
	var prev string
	for i, l := range FindLeaks(code, values) {
		if i > 0 && l.Text == prev {
			continue
		}
		prev = l.Text
		out = append(out, l.Value)
	}
	return out
}

// ExampleValues は examples の in / out の値を平らにして返す（配列・オブジェクトは中の値。null は除く）。
func ExampleValues(exs []Example) []any {
	var vs []any
	for _, ex := range exs {
		vs = append(vs, ex.In, ex.Out)
	}
	return flatten(vs)
}

// LeakTarget はプロジェクトルート相対の path が漏れ検知の対象か。
// テストファイル（_test.go / .test.ts / .spec.ts）とルート直下の claims/・holdout/・docs/ は対象外。
func LeakTarget(p string) bool {
	p = strings.TrimPrefix(path.Clean(strings.ReplaceAll(p, `\`, "/")), "./")
	for _, suf := range []string{"_test.go", ".test.ts", ".spec.ts"} {
		if strings.HasSuffix(p, suf) {
			return false
		}
	}
	for _, dir := range []string{"claims/", "holdout/", "docs/"} {
		if strings.HasPrefix(p, dir) {
			return false
		}
	}
	return true
}

// leakText は値を探す表記にする。数値は JSON の表記（小数は書かれたとおりの小数の表記で探す）。
func leakText(v any) (text string, isNum, ok bool) {
	switch x := normalizeNumber(v).(type) {
	case string:
		return x, false, utf8.RuneCountInString(x) >= leakMinRunes
	case float64:
		b, err := json.Marshal(x)
		if err != nil {
			return "", false, false
		}
		digits := 0
		for _, c := range string(b) {
			if c >= '0' && c <= '9' {
				digits++
			}
		}
		return string(b), true, digits >= leakMinDigits
	}
	return "", false, false
}

// occurrences は text が現れるバイト位置。数値は前後が数字・小数点・英字でない位置だけ
// （値 N は N.5 の中の N、N の後ろに数字が続く場所、英字の直後の N を数えない）。
func occurrences(code, text string, isNum bool) []int {
	var offs []int
	for start := 0; start < len(code); {
		i := strings.Index(code[start:], text)
		if i < 0 {
			break
		}
		off := start + i
		end := off + len(text)
		if !isNum || (off == 0 || !numberAdjacent(code[off-1])) && (end == len(code) || !numberAdjacent(code[end])) {
			offs = append(offs, off)
		}
		start = off + 1
	}
	return offs
}

func numberAdjacent(c byte) bool {
	return c >= '0' && c <= '9' || c == '.' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// flatten は配列・オブジェクトを中の値に展開する。オブジェクトはキー順（出力を決定的にする）。null は除く。
func flatten(values []any) []any {
	var out []any
	for _, v := range values {
		switch x := v.(type) {
		case nil:
		case []any:
			out = append(out, flatten(x)...)
		case map[string]any:
			keys := make([]string, 0, len(x))
			for k := range x {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				out = append(out, flatten([]any{x[k]})...)
			}
		default:
			out = append(out, v)
		}
	}
	return out
}
