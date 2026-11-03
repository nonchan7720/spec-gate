package extractor

import (
	"os"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// 委譲用のプロンプトは prompt.md のルールを省略せずに含み、ref と抜粋を埋める。
func TestPrompter_PromptContainsFullRulesAndExcerpt(t *testing.T) {
	tmpl, err := os.ReadFile("prompt.md")
	if err != nil {
		t.Fatal(err)
	}
	got := Prompter{}.Prompt("docs/spec/a.md#端数", "円未満は切り捨てる")
	for line := range strings.SplitSeq(string(tmpl), "\n") {
		if strings.Contains(line, "{{") {
			continue
		}
		if !strings.Contains(got, line) {
			t.Fatalf("prompt.md の行 %q が無い", line)
		}
	}
	for _, s := range []string{"docs/spec/a.md#端数", "円未満は切り捨てる"} {
		if !strings.Contains(got, s) {
			t.Fatalf("%q が無い:\n%s", s, got)
		}
	}
	if strings.Contains(got, "{{") {
		t.Fatalf("置換し残し:\n%s", got)
	}
}

func TestPrompter_Parse(t *testing.T) {
	answer := "抽出しました。\n```yaml\n- id: R-1\n  kind: example\n  title: \"切り捨て\"\n  source:\n    type: markdown\n    ref: other.md\n  examples:\n    - { in: 1.5, out: 1 }\n```\n"
	claims, err := Prompter{}.Parse(answer, "a.md#x")
	if err != nil {
		t.Fatal(err)
	}
	if len(claims) != 1 || claims[0].ID != "R-1" || claims[0].Kind != domain.KindExample || claims[0].Source.Ref != "a.md#x" {
		t.Fatalf("claims = %+v", claims)
	}
	// フェンスが無くても YAML の配列なら読む
	claims, err = Prompter{}.Parse("- id: R-2\n  kind: manual\n  title: \"x\"\n", "a.md#x")
	if err != nil || len(claims) != 1 || claims[0].ID != "R-2" {
		t.Fatalf("フェンスなし: %+v %v", claims, err)
	}
	if _, err := (Prompter{}).Parse("答えられません", "a.md#x"); err == nil {
		t.Fatal("claim が読めなければ error")
	}
}
