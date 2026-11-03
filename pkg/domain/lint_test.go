package domain

import (
	"strings"
	"testing"
)

func exampleClaim(id string, ex []Example, props []string) Claim {
	return Claim{
		ID: id, Kind: KindExample, Title: id,
		Source:  Source{Type: SourceMarkdown, Ref: "spec.md#x"},
		Binding: "cmd", Examples: ex, Properties: props,
	}
}

func TestLint_ExampleViolatesProperty(t *testing.T) {
	c := exampleClaim("A-1", []Example{{In: 1.5, Out: 1.5}}, []string{"isInteger(output)"})
	issues := Lint([]Claim{c})
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "満たさない") {
		t.Fatalf("property 違反を検出すべき: %+v", issues)
	}
}

func TestLint_ConflictBetweenClaims(t *testing.T) {
	a := exampleClaim("A-1", []Example{{In: 10, Out: 10}}, nil)
	b := exampleClaim("A-2", []Example{{In: 10, Out: 11}}, nil)
	issues := Lint([]Claim{a, b})
	if len(issues) != 1 || issues[0].ClaimID != "A-2" || !strings.Contains(issues[0].Message, "矛盾") {
		t.Fatalf("claim 間の矛盾を検出すべき: %+v", issues)
	}
}

func TestLint_IntAndFloatAreEqual(t *testing.T) {
	// YAML は 1234 を int、1234.0 を float で読むが同値として扱う
	if !Equal(1234, 1234.0) {
		t.Fatal("1234 と 1234.0 は等しいはず")
	}
}

func TestLint_InvalidExpression(t *testing.T) {
	c := exampleClaim("A-1", []Example{{In: 1, Out: 1}}, []string{"Number.isInteger(output)"})
	issues := Lint([]Claim{c})
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "評価できない") {
		t.Fatalf("不正な式を報告すべき: %+v", issues)
	}
}

func TestPlan_VerifyHashIgnoresTextAndDone(t *testing.T) {
	p1 := Plan{Items: []PlanItem{{Text: "a", Verify: "true", Done: false}}}
	p2 := Plan{Items: []PlanItem{{Text: "b", Verify: "true", Done: true}}}
	p3 := Plan{Items: []PlanItem{{Text: "a", Verify: "false"}}}
	if p1.VerifyHash() != p2.VerifyHash() {
		t.Fatal("文言・チェック状態の変更でハッシュが変わってはいけない")
	}
	if p1.VerifyHash() == p3.VerifyHash() {
		t.Fatal("verify の変更でハッシュが変わるべき")
	}
}

func TestStatus_Blocking(t *testing.T) {
	for s, want := range map[Status]bool{
		StatusPass: false, StatusUnverified: false,
		StatusFail: true, StatusStale: true, StatusUnapproved: true,
	} {
		if s.Blocking() != want {
			t.Errorf("%s.Blocking() = %v, want %v", s, !want, want)
		}
	}
}
