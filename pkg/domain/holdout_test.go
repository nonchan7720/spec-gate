package domain

import (
	"strings"
	"testing"
)

func TestHoldoutSummary(t *testing.T) {
	cases := map[string]struct {
		in   []HoldoutResult
		want string
	}{
		"すべて一致": {[]HoldoutResult{{Pass: true}, {Pass: true}}, "holdout 2 件すべて一致"},
		"不一致あり": {[]HoldoutResult{{Pass: true}, {Pass: false}}, "holdout 2 件中 1 件が不一致（値は表示しない）"},
		"なし":    {nil, "holdout なし"},
	}
	for label, c := range cases {
		t.Run(label, func(t *testing.T) {
			if got := HoldoutSummary(c.in); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

const (
	// hash1234 は 1234 の正規化 JSON（"1234"）の SHA-256
	hash1234 = "03ac674216f3e15c761ee1a5e255f067953623c8b388b4459e13f978d7c846f4"
	// hashObj は {"a":[1,2.5,"<&>"],"b":1} の SHA-256
	hashObj = "096e177885037b67c685e0c5608fa34efad5a1a64e2bc43d7db8d679dc123e69"
)

func TestCanonicalJSON(t *testing.T) {
	cases := map[string]struct {
		in   any
		want string
	}{
		"キーは辞書順・HTML エスケープなし":      {map[string]any{"b": 1, "a": []any{1, 2.5, "<&>"}}, `{"a":[1,2.5,"<&>"],"b":1}`},
		"YAML の int は float64 と同じ": {1234, "1234"},
		"float64":                  {1234.0, "1234"},
		"null":                     {nil, "null"},
		"文字列":                      {"円未満", `"円未満"`},
	}
	for label, c := range cases {
		t.Run(label, func(t *testing.T) {
			got, err := CanonicalJSON(c.in)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestOutputHash(t *testing.T) {
	for _, c := range []struct {
		in   any
		want string
	}{
		{1234, hash1234},
		{1234.0, hash1234},
		{map[string]any{"b": 1.0, "a": []any{1, 2.5, "<&>"}}, hashObj},
	} {
		got, err := OutputHash(c.in)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Fatalf("OutputHash(%v) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestValidOutputHash(t *testing.T) {
	if !ValidOutputHash(hash1234) {
		t.Fatal("16 進小文字 64 文字は正しい")
	}
	for _, s := range []string{"", "abc", strings.ToUpper(hash1234), hash1234[:63] + "g", hash1234 + "0"} {
		if ValidOutputHash(s) {
			t.Fatalf("%q は正しくない", s)
		}
	}
}

func TestHoldoutInputValues(t *testing.T) {
	got := HoldoutInputValues([]HoldoutExample{{In: 31337.5, OutSHA256: hash1234}, {In: []any{"abcd", nil}, OutSHA256: hash1234}})
	if len(got) != 2 || !Equal(got[0], 31337.5) || got[1] != "abcd" {
		t.Fatalf("holdout は in の値だけを返す: %v", got)
	}
}

func TestLintHoldout_ShapeOnly(t *testing.T) {
	c := Claim{ID: "ROUND-001", Kind: KindExample, Title: "t", Source: Source{Type: SourceMarkdown, Ref: "a.md"},
		Binding: "x", Examples: []Example{{In: 1.5, Out: 1}}, Properties: []string{"output <= input"}}
	holdout := map[string][]HoldoutExample{"ROUND-001": {{In: 7777.25, OutSHA256: hash1234}, {In: 3.5, OutSHA256: "7777"}}}

	issues := LintHoldout([]Claim{c}, holdout)
	if len(issues) != 1 {
		t.Fatalf("形の崩れた holdout だけを報告する（properties は見ない）: %+v", issues)
	}
	if issues[0].ClaimID != "ROUND-001" || !strings.Contains(issues[0].Message, "holdout[1]") || !strings.Contains(issues[0].Message, "out_sha256") {
		t.Fatalf("どの holdout かは番号で示す: %+v", issues[0])
	}
	for _, v := range []string{"3.5", "7777"} {
		if strings.Contains(issues[0].Message, v) {
			t.Fatalf("holdout の値 %s を出してはならない: %q", v, issues[0].Message)
		}
	}
}

func TestLintHoldout_OrphanAndNonExample(t *testing.T) {
	m := Claim{ID: "M", Kind: KindManual, Title: "t", Source: Source{Type: SourceMarkdown, Ref: "a.md"}}
	holdout := map[string][]HoldoutExample{"M": {{In: 1, OutSHA256: hash1234}}, "NONE": {{In: 1, OutSHA256: hash1234}}}
	issues := LintHoldout([]Claim{m}, holdout)
	got := map[string]bool{}
	for _, is := range issues {
		got[is.ClaimID] = true
	}
	if len(issues) != 2 || !got["M"] || !got["NONE"] {
		t.Fatalf("claim の無い holdout と、example 以外の claim の holdout は使われないので報告する: %+v", issues)
	}
}

func TestLintHoldout_EmptyIsFine(t *testing.T) {
	c := Claim{ID: "A", Kind: KindExample, Title: "t", Source: Source{Type: SourceMarkdown, Ref: "a.md"},
		Binding: "x", Examples: []Example{{In: 1, Out: 1}}, Properties: []string{"output <= input"}}
	if issues := LintHoldout([]Claim{c}, nil); len(issues) != 0 {
		t.Fatalf("holdout が無ければ何も出さない: %+v", issues)
	}
}
