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

func TestPrompter_ParseNormalizesJSONStringExamples(t *testing.T) {
	answer := "```yaml\n- id: C-1\n  kind: example\n  title: \"t\"\n  binding: go run ./tools/bind touched\n  hint: \"\"\n  examples:\n    - in: '{\"touches\": [\"a.ts\"], \"changed\": \"a.ts\"}'\n      out: true\n    - in: \"123\"\n      out: '[1, 2]'\n```"
	claims, err := Prompter{}.Parse(answer, "a.md#x")
	if err != nil {
		t.Fatal(err)
	}
	ex := claims[0].Examples
	if m, ok := ex[0].In.(map[string]any); !ok || m["changed"] != "a.ts" {
		t.Errorf("JSON 文字列のオブジェクトは構造に戻す: %#v", ex[0].In)
	}
	if ex[1].In != "123" {
		t.Errorf("数値に読める文字列は変えない: %#v", ex[1].In)
	}
	if arr, ok := ex[1].Out.([]any); !ok || len(arr) != 2 {
		t.Errorf("out も構造に戻す: %#v", ex[1].Out)
	}
}

func TestPrompter_PromptAsksForHintAndStructuredValues(t *testing.T) {
	p := Prompter{}.Prompt("a.md#x", "本文")
	for _, want := range []string{"hint", "JSON の文字列にしない"} {
		if !strings.Contains(p, want) {
			t.Errorf("プロンプトに %q が必要", want)
		}
	}
}
