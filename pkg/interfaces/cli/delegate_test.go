package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

const delegSpec = "# 金額計算\n\n## 端数処理\n\n円未満は切り捨てる。\n\n## マイナス金額\n\n0 にする。\n"

func writeFile(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newDelegProject はテンポラリのプロジェクト（仕様 1 つ）と、そこを root にした app を作る。
// LLM コマンドの環境変数は空にする（テストで claude を呼ばない）。
func newDelegProject(t *testing.T) (string, *app) {
	t.Helper()
	t.Setenv("SPEC_GATE_EXTRACT_CMD", "")
	t.Setenv("SPEC_GATE_REVERSE_CMD", "")
	root := t.TempDir()
	writeFile(t, root, "docs/spec/a.md", delegSpec)
	return root, newApp(root)
}

func runCmd(t *testing.T, cmd *cobra.Command, args ...string) (string, string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

var answerLine = regexp.MustCompile(`回答ファイル: (\S+)`)

// answerPath はプロンプトファイルに書かれた回答ファイルのパスを返す。
func answerPath(t *testing.T, prompt string) string {
	t.Helper()
	b, err := os.ReadFile(prompt)
	if err != nil {
		t.Fatal(err)
	}
	m := answerLine.FindStringSubmatch(string(b))
	if m == nil {
		t.Fatalf("%s に回答ファイルのパスが無い", prompt)
	}
	return m[1]
}

func lines(s string) []string {
	var out []string
	for l := range strings.SplitSeq(strings.TrimSpace(s), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// --emit / --collect も環境変数も無ければ、LLM を呼ばずに委譲の手順を案内してエラーにする。
func TestExtract_NoModeExplainsDelegation(t *testing.T) {
	root, a := newDelegProject(t)
	_, _, err := runCmd(t, newExtractCmd(&a), "docs/spec/a.md#端数処理")
	if err == nil {
		t.Fatal("error にする")
	}
	for _, s := range []string{"--emit", "--collect", "spec-gate-llm-worker", "spec-gate-extract", "SPEC_GATE_EXTRACT_CMD"} {
		if !strings.Contains(err.Error(), s) {
			t.Fatalf("案内に %q が無い:\n%v", s, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".spec-gate-llm")); !os.IsNotExist(err) {
		t.Fatal("何も書かない")
	}
}

func TestReverse_NoModeExplainsDelegation(t *testing.T) {
	_, a := newDelegProject(t)
	_, _, err := runCmd(t, newReverseCmd(&a))
	if err == nil {
		t.Fatal("error にする")
	}
	for _, s := range []string{"--emit", "--collect", "spec-gate-llm-worker", "spec-gate-reverse", "SPEC_GATE_REVERSE_CMD"} {
		if !strings.Contains(err.Error(), s) {
			t.Fatalf("案内に %q が無い:\n%v", s, err)
		}
	}
}

func TestExtract_EmitAndCollectTogetherIsError(t *testing.T) {
	_, a := newDelegProject(t)
	if _, _, err := runCmd(t, newExtractCmd(&a), "docs/spec/a.md#端数処理", "--emit", "--collect"); err == nil {
		t.Fatal("--emit と --collect は同時に使えない")
	}
}

// emit → （サブエージェントの代わりに決まった回答を書く）→ collect の一巡。
func TestExtract_EmitThenCollect(t *testing.T) {
	root, a := newDelegProject(t)
	out, _, err := runCmd(t, newExtractCmd(&a), "docs/spec/a.md#端数処理", "--runs", "2", "--emit")
	if err != nil {
		t.Fatal(err)
	}
	prompts := lines(out)
	want := []string{
		filepath.Join(root, ".spec-gate-llm/extract/docs_spec_a.md_端数処理/run-1.prompt.md"),
		filepath.Join(root, ".spec-gate-llm/extract/docs_spec_a.md_端数処理/run-2.prompt.md"),
	}
	if strings.Join(prompts, "\n") != strings.Join(want, "\n") {
		t.Fatalf("stdout はプロンプトのパスだけ:\n%s", out)
	}
	body, _ := os.ReadFile(prompts[0])
	for _, s := range []string{"円未満は切り捨てる。", "機械的に検証できる主張", "ルール:"} {
		if !strings.Contains(string(body), s) {
			t.Fatalf("プロンプトに %q が無い（自己完結していない）", s)
		}
	}

	// collect は回答が欠けていればエラー
	if _, _, err := runCmd(t, newExtractCmd(&a), "docs/spec/a.md#端数処理", "--collect"); err == nil || !strings.Contains(err.Error(), "run-1") {
		t.Fatalf("欠けた run を挙げる: %v", err)
	}

	answer := "```yaml\n- id: ROUND-001\n  kind: manual\n  title: \"円未満は切り捨てる\"\n  source:\n    type: markdown\n    ref: docs/spec/a.md#端数処理\n```\n"
	for _, p := range prompts {
		writeFile(t, "/", answerPath(t, p), answer)
	}
	out, _, err = runCmd(t, newExtractCmd(&a), "docs/spec/a.md#端数処理", "--collect")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "2/2") || !strings.Contains(out, "ROUND-001") {
		t.Fatalf("合議の結果を出す:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(root, "claims/_pending/ROUND-001.yaml")); err != nil {
		t.Fatalf("_pending に置く: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".spec-gate-llm")); !os.IsNotExist(err) {
		t.Fatal("collect 後は作業ディレクトリを消す")
	}
}

const delegClaim = `id: ROUND-001
kind: manual
title: 円未満は切り捨て
source:
    type: markdown
    ref: docs/spec/a.md#端数処理
`

// reverse の emit → 再生成の回答 → collect（比較のプロンプト）→ 比較の回答 → collect（レポート）。
func TestReverse_EmitThenCollectTwoPhases(t *testing.T) {
	root, a := newDelegProject(t)
	writeFile(t, root, "claims/spec/ROUND-001.yaml", delegClaim)

	out, _, err := runCmd(t, newReverseCmd(&a), "--emit")
	if err != nil {
		t.Fatal(err)
	}
	restate := lines(out)
	if len(restate) != 1 || restate[0] != filepath.Join(root, ".spec-gate-llm/reverse/ROUND-001.restate.prompt.md") {
		t.Fatalf("再生成のプロンプト:\n%s", out)
	}
	body, _ := os.ReadFile(restate[0])
	if strings.Contains(string(body), "円未満は切り捨てる。") {
		t.Fatal("再生成のプロンプトに元の仕様を入れない")
	}
	writeFile(t, "/", answerPath(t, restate[0]), "円未満は切り捨てる。\n")

	out, _, err = runCmd(t, newReverseCmd(&a), "--collect")
	if err != nil {
		t.Fatal(err)
	}
	compare := lines(out)
	if len(compare) != 1 || compare[0] != filepath.Join(root, ".spec-gate-llm/reverse/ROUND-001.compare.prompt.md") {
		t.Fatalf("比較のプロンプト:\n%s", out)
	}
	writeFile(t, "/", answerPath(t, compare[0]), `{"match": true, "differences": []}`)

	out, _, err = runCmd(t, newReverseCmd(&a), "--collect")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "✓ ROUND-001") || !strings.Contains(out, "1 match, 0 mismatch") {
		t.Fatalf("レポート:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(root, ".spec-gate-llm")); !os.IsNotExist(err) {
		t.Fatal("レポート後は作業ディレクトリを消す")
	}
}
