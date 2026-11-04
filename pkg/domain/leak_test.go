package domain

import (
	"encoding/json"
	"testing"
)

func jsonValues(t *testing.T, s string) []any {
	t.Helper()
	var v []any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// docs/spec/harness.md「リテラル漏れ検知」の表
func TestLeakedValues_SpecTable(t *testing.T) {
	cases := []struct{ code, values, want string }{
		{"if x == 1234.56 { return 1234 }", `[1234.56, 1234]`, `[1234.56,1234]`},
		{"return math.Floor(x)", `[1234.56, 1234]`, `[]`},
		{"return 0", `[0, 1]`, `[]`},
		{`msg := "円未満は切り捨て"`, `["円未満は切り捨て", "ok"]`, `["円未満は切り捨て"]`},
	}
	for _, c := range cases {
		got, _ := json.Marshal(LeakedValues(c.code, jsonValues(t, c.values)))
		if string(got) != c.want {
			t.Fatalf("code=%q values=%s: got %s, want %s", c.code, c.values, got, c.want)
		}
	}
}

func TestLeakedValues_NumberRules(t *testing.T) {
	cases := []struct {
		label, code string
		value       any
		want        bool
	}{
		{"1234.56 の中の 1234 は数えない", "x := 1234.56", 1234.0, false},
		{"12345 の中の 1234 は数えない", "x := 12345", 1234.0, false},
		{"0.1234 の中の 1234 は数えない", "x := 0.1234", 1234.0, false},
		{"英字に続く 1234 は数えない", "v1234", 1234.0, false},
		{"英字が続く 1234 は数えない", "1234px", 1234.0, false},
		{"記号に挟まれた 1234 は数える", "f(1234)", 1234.0, true},
		{"行末の 1234 は数える", "return 1234", 1234.0, true},
		{"文末の小数点（数字が続かない）は小数点として扱う", "x is 1234.", 1234.0, false},
		{"1.25 は 3 桁なので対象", "y := 1.25", 1.25, true},
		{"-12 は 2 桁なので対象外", "y := -12", -12.0, false},
		{"-123 は 3 桁なので対象", "y := -123", -123.0, true},
		{"10 は 2 桁なので対象外", "y := 10", 10.0, false},
		{"100 は 3 桁なので対象", "y := 100", 100.0, true},
		{"YAML の int も数値として扱う", "y := 1234", 1234, true},
		{"表記が違えば数えない（1234.50）", "y := 1234.50", 1234.5, false},
	}
	for _, c := range cases {
		got := len(LeakedValues(c.code, []any{c.value})) == 1
		if got != c.want {
			t.Errorf("%s: code=%q value=%v got %v", c.label, c.code, c.value, got)
		}
	}
}

func TestLeakedValues_StringsAndOthers(t *testing.T) {
	cases := []struct {
		label, code string
		value       any
		want        bool
	}{
		{"4 文字以上の文字列", `s := "abcd"`, "abcd", true},
		{"3 文字の文字列は対象外", `s := "abc"`, "abc", false},
		{"文字数は rune で数える", `s := "切り捨て"`, "切り捨て", true},
		{"真偽値は対象外", "return true", true, false},
		{"null は対象外", "return nil", nil, false},
	}
	for _, c := range cases {
		got := len(LeakedValues(c.code, []any{c.value})) == 1
		if got != c.want {
			t.Errorf("%s: got %v", c.label, got)
		}
	}
}

func TestLeakedValues_DedupAndNested(t *testing.T) {
	got, _ := json.Marshal(LeakedValues("a := 1234; b := 1234 // 支払一覧", []any{1234.0, []any{1234.0, "支払一覧"}, map[string]any{"k": 5678.0}}))
	if string(got) != `[1234,"支払一覧"]` {
		t.Fatalf("配列・オブジェクトの中の値も調べ、同じ値は 1 回だけ返す: %s", got)
	}
}

func TestFindLeaks_Lines(t *testing.T) {
	leaks := FindLeaks("package x\n\nconst a = 1234\nconst b = 1234 + 5678\n", []any{1234.0, 5678.0})
	want := []Leak{{Value: 1234.0, Text: "1234", Line: 3}, {Value: 1234.0, Text: "1234", Line: 4}, {Value: 5678.0, Text: "5678", Line: 4}}
	if len(leaks) != len(want) {
		t.Fatalf("値と行ごとに返す: %+v", leaks)
	}
	for i := range want {
		if leaks[i] != want[i] {
			t.Fatalf("[%d] got %+v, want %+v", i, leaks[i], want[i])
		}
	}
}

func TestExampleValues(t *testing.T) {
	got := ExampleValues([]Example{{In: []any{1234, "abcd"}, Out: map[string]any{"x": 5.5}}, {In: "efgh", Out: nil}})
	b, _ := json.Marshal(got)
	if string(b) != `[1234,"abcd",5.5,"efgh"]` {
		t.Fatalf("in / out の値を平らにして返す（null は除く）: %s", b)
	}
}

func TestLeakTarget(t *testing.T) {
	cases := map[string]bool{
		"src/payroll/round.ts":         true,
		"pkg/services/check.go":        true,
		"pkg/services/check_test.go":   false,
		"src/payroll/round.test.ts":    false,
		"src/payroll/round.spec.ts":    false,
		"claims/spec/ROUND-001.yaml":   false,
		"holdout/ROUND-001.yaml":       false,
		"docs/spec/rounding.md":        false,
		"./docs/spec/rounding.md":      false,
		"pkg/docs/x.go":                true, // ルート直下の docs/ だけが対象外
		"src/claimsview/List.tsx":      true,
		"src/payroll/round.testing.ts": true,
	}
	for p, want := range cases {
		if got := LeakTarget(p); got != want {
			t.Errorf("%s: got %v, want %v", p, got, want)
		}
	}
}
