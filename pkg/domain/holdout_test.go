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

func TestLintHoldout_ReportsWithoutValues(t *testing.T) {
	c := Claim{ID: "ROUND-001", Kind: KindExample, Title: "t", Source: Source{Type: SourceMarkdown, Ref: "a.md"},
		Binding: "x", Examples: []Example{{In: 1.5, Out: 1}}, Properties: []string{"output <= input"}}
	holdout := map[string][]Example{"ROUND-001": {{In: 7777.25, Out: 7777}, {In: 3.5, Out: 9999}}}

	issues := LintHoldout([]Claim{c}, holdout)
	if len(issues) != 1 {
		t.Fatalf("満たさない holdout は 1 件: %+v", issues)
	}
	if issues[0].ClaimID != "ROUND-001" || !strings.Contains(issues[0].Message, "holdout[1]") {
		t.Fatalf("どの holdout かは番号で示す: %+v", issues[0])
	}
	for _, v := range []string{"3.5", "9999", "7777"} {
		if strings.Contains(issues[0].Message, v) {
			t.Fatalf("holdout の値 %s を出してはならない: %q", v, issues[0].Message)
		}
	}
}

func TestLintHoldout_UnevaluablePropertyHidesError(t *testing.T) {
	c := Claim{ID: "A", Kind: KindExample, Title: "t", Source: Source{Type: SourceMarkdown, Ref: "a.md"},
		Binding: "x", Properties: []string{"output <= input"}}
	holdout := map[string][]Example{"A": {{In: "secret-text", Out: 1}}}
	issues := LintHoldout([]Claim{c}, holdout)
	if len(issues) != 1 {
		t.Fatalf("評価できない property も報告する: %+v", issues)
	}
	if strings.Contains(issues[0].Message, "secret-text") || strings.Contains(issues[0].Message, "string") {
		t.Fatalf("評価エラーの本文（値や型）も出さない: %q", issues[0].Message)
	}
}

func TestLintHoldout_OrphanAndNonExample(t *testing.T) {
	m := Claim{ID: "M", Kind: KindManual, Title: "t", Source: Source{Type: SourceMarkdown, Ref: "a.md"}}
	holdout := map[string][]Example{"M": {{In: 1, Out: 2}}, "NONE": {{In: 1, Out: 2}}}
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
