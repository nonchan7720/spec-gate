package services

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// 仕様「LLM 処理のサブエージェント委譲」の表どおりのパスになる。
func TestExtractPromptPaths_SpecTable(t *testing.T) {
	cases := []struct {
		ref  string
		runs int
		want []string
	}{
		{"docs/spec/rounding.md#端数処理", 2, []string{
			".spec-gate-llm/extract/docs_spec_rounding.md_端数処理/run-1.prompt.md",
			".spec-gate-llm/extract/docs_spec_rounding.md_端数処理/run-2.prompt.md",
		}},
		{"docs/arch/layers.mmd", 1, []string{".spec-gate-llm/extract/docs_arch_layers.mmd/run-1.prompt.md"}},
	}
	for _, c := range cases {
		if got := ExtractPromptPaths(c.ref, c.runs); !reflect.DeepEqual(got, c.want) {
			t.Errorf("ExtractPromptPaths(%q, %d) = %v, want %v", c.ref, c.runs, got, c.want)
		}
	}
}

// 空白（全角・タブを含む）も _ に置き換える。
func TestLLMName_ReplacesSlashHashAndWhitespace(t *testing.T) {
	if got := llmName("a b/c#d\te　f"); got != "a_b_c_d_e_f" {
		t.Fatalf("llmName = %q", got)
	}
}

// fakeWork は作業ディレクトリをメモリ上に持つ。
type fakeWork struct {
	files   map[string]string
	removed []string
}

func newFakeWork() *fakeWork { return &fakeWork{files: map[string]string{}} }

func (w *fakeWork) Write(rel, content string) error {
	w.files[rel] = content
	return nil
}

func (w *fakeWork) Read(rel string) (string, bool, error) {
	s, ok := w.files[rel]
	return s, ok, nil
}

func (w *fakeWork) Abs(rel string) string { return "/abs/" + rel }

func (w *fakeWork) RemoveAll(rel string) error {
	w.removed = append(w.removed, rel)
	for k := range w.files {
		if strings.HasPrefix(k, rel+"/") {
			delete(w.files, k)
		}
	}
	return nil
}

// under は dir 直下の（ファイル名でソートした）ファイル一覧。
func (w *fakeWork) under(dir string) []string {
	var out []string
	for k := range w.files {
		if name, ok := strings.CutPrefix(k, dir+"/"); ok {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// fakeExtractPrompter は抽出プロンプトを決まった形で組み立て、回答は "id|title" の行として読む。
type fakeExtractPrompter struct{}

func (fakeExtractPrompter) Prompt(ref, excerpt string) string {
	return fmt.Sprintf("RULES(full)\nREF=%s\nSPEC=%s", ref, excerpt)
}

func (fakeExtractPrompter) Parse(answer, ref string) ([]domain.Claim, error) {
	var out []domain.Claim
	for line := range strings.SplitSeq(strings.TrimSpace(answer), "\n") {
		id, title, ok := strings.Cut(line, "|")
		if !ok {
			return nil, fmt.Errorf("読めない行 %q", line)
		}
		out = append(out, domain.Claim{ID: id, Kind: domain.KindManual, Title: title, Source: domain.Source{Ref: ref}})
	}
	return out, nil
}
