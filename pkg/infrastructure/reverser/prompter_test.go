package reverser

import (
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// 再生成のプロンプトは prompt_restate.md の全文と claim の内容を含み、元の仕様の位置は含めない。
func TestPrompter_RestatePrompt(t *testing.T) {
	c := domain.Claim{ID: "R-1", Kind: domain.KindExample, Title: "端数は切り捨て",
		Source:   domain.Source{Type: domain.SourceMarkdown, Ref: "docs/spec/secret.md#端数"},
		Binding:  "npx tsx tools/call.ts",
		Examples: []domain.Example{{In: 1.5, Out: 1}}}
	got, err := Prompter{}.RestatePrompt(c)
	if err != nil {
		t.Fatal(err)
	}
	assertTemplateLines(t, restatePrompt, got)
	for _, s := range []string{"端数は切り捨て", "1.5"} {
		if !strings.Contains(got, s) {
			t.Fatalf("%q が無い:\n%s", s, got)
		}
	}
	for _, s := range []string{"secret.md", "tools/call.ts"} {
		if strings.Contains(got, s) {
			t.Fatalf("再生成に %q を渡さない:\n%s", s, got)
		}
	}
}

func TestPrompter_ComparePrompt(t *testing.T) {
	got := Prompter{}.ComparePrompt("再生成した文", "元の仕様")
	assertTemplateLines(t, comparePrompt, got)
	if !strings.Contains(got, "再生成した文") || !strings.Contains(got, "元の仕様") {
		t.Fatalf("A / B が無い:\n%s", got)
	}
}

func TestPrompter_ParseAnswers(t *testing.T) {
	p := Prompter{}
	if s, err := p.ParseRestate("```\n円未満は切り捨てる。\n```\n"); err != nil || s != "円未満は切り捨てる。" {
		t.Fatalf("ParseRestate = %q %v", s, err)
	}
	if _, err := p.ParseRestate("  \n"); err == nil {
		t.Fatal("空の再生成は error")
	}
	cmp, err := p.ParseCompare(`{"match": false, "differences": ["負数"]}`)
	if err != nil || cmp.Match || len(cmp.Differences) != 1 {
		t.Fatalf("ParseCompare = %+v %v", cmp, err)
	}
	if _, err := p.ParseCompare("わかりません"); err == nil {
		t.Fatal("JSON が無ければ error")
	}
}

func assertTemplateLines(t *testing.T, tmpl, got string) {
	t.Helper()
	for line := range strings.SplitSeq(tmpl, "\n") {
		if strings.Contains(line, "{{") {
			continue
		}
		if !strings.Contains(got, line) {
			t.Fatalf("テンプレートの行 %q が無い", line)
		}
	}
	if strings.Contains(got, "{{") {
		t.Fatalf("置換し残し:\n%s", got)
	}
}
