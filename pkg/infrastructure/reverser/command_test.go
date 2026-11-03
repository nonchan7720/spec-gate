package reverser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

func TestParseComparison(t *testing.T) {
	cases := []struct {
		name  string
		out   string
		match bool
		diffs []string
	}{
		{"素の JSON", `{"match": true, "differences": []}`, true, nil},
		{"json フェンス", "```json\n{\"match\": false, \"differences\": [\"負数の扱いが claim に無い\"]}\n```\n", false, []string{"負数の扱いが claim に無い"}},
		{"言語名なしのフェンス", "```\n{\"match\": true}\n```", true, nil},
		{"前後に説明文", "比較しました。\n{\"match\": false, \"differences\": [\"a\", \"b\"]}\n以上です。", false, []string{"a", "b"}},
		{"説明文に波括弧", "結果 {一致しない} は次のとおり:\n{\"match\": false, \"differences\": [\"x\"]}", false, []string{"x"}},
		{"文字列中の波括弧", `{"match": false, "differences": ["式 {a} が違う"]}`, false, []string{"式 {a} が違う"}},
		// match=true なのに食い違いを挙げている応答は矛盾している。一致とはみなさない
		{"match=true でも差分があれば不一致", `{"match": true, "differences": ["丸め方向が違う"]}`, false, []string{"丸め方向が違う"}},
		{"空文字の差分は捨てる", `{"match": true, "differences": ["", "  "]}`, true, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseComparison(tc.out)
			if err != nil {
				t.Fatal(err)
			}
			if got.Match != tc.match || strings.Join(got.Differences, "|") != strings.Join(tc.diffs, "|") {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestParseComparison_BrokenIsError(t *testing.T) {
	for name, out := range map[string]string{
		"空":             "",
		"JSON が無い":      "一致しています。",
		"壊れた JSON":      `{"match": true, "differences": [`,
		"match が無い":     `{"differences": []}`,
		"match が真偽値でない": `{"match": "yes"}`,
		"閉じていない":        "```json\n{\"match\": true\n```",
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := ParseComparison(out); err == nil {
				t.Fatalf("解釈できない応答は error（一致にしない）: %+v", got)
			}
		})
	}
}

// fakeCommand は stdin を prompt ファイルに保存し、canned を出力するスクリプトを作る。
func fakeCommand(t *testing.T, canned string) (command, promptFile string) {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(out, []byte(canned), 0o644); err != nil {
		t.Fatal(err)
	}
	promptFile = filepath.Join(dir, "prompt.txt")
	script := filepath.Join(dir, "fake.sh")
	body := "#!/bin/sh\ncat > '" + promptFile + "'\ncat '" + out + "'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return script, promptFile
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func roundClaim() domain.Claim {
	return domain.Claim{
		ID: "ROUND-001", Kind: domain.KindExample, Title: "円未満は切り捨て",
		Source:     domain.Source{Type: domain.SourceMarkdown, Ref: "docs/spec/rounding.md#端数処理"},
		Approved:   &domain.Approval{By: "alice", SourceHash: "sha256:secret-hash"},
		Binding:    "npx tsx tools/call.ts src/round.ts roundYen",
		Examples:   []domain.Example{{In: 1234.56, Out: 1234}},
		Properties: []string{"isInteger(output)"},
	}
}

func TestCommand_Restate(t *testing.T) {
	script, prompt := fakeCommand(t, "```\n金額の円未満は切り捨てる。1234.56 円は 1234 円になる。\n```\n")
	r := Command{Command: script, Dir: t.TempDir()}
	got, err := r.Restate(context.Background(), roundClaim())
	if err != nil {
		t.Fatal(err)
	}
	if got != "金額の円未満は切り捨てる。1234.56 円は 1234 円になる。" {
		t.Fatalf("フェンスと空白を取り除いた再生成文: %q", got)
	}
	p := readFile(t, prompt)
	for _, want := range []string{"円未満は切り捨て", "1234.56", "isInteger(output)", "example"} {
		if !strings.Contains(p, want) {
			t.Fatalf("prompt に claim の内容 %q が無い:\n%s", want, p)
		}
	}
	// 元の仕様の位置・承認情報・binding は見せない（claim の主張だけから書き起こさせる）
	for _, deny := range []string{"docs/spec/rounding.md", "sha256:secret-hash", "tools/call.ts", "{{"} {
		if strings.Contains(p, deny) {
			t.Fatalf("prompt に %q が含まれている:\n%s", deny, p)
		}
	}
}

func TestCommand_RestateStructureAndUI(t *testing.T) {
	script, prompt := fakeCommand(t, "仕様文")
	r := Command{Command: script}
	c := domain.Claim{
		ID: "ARCH-001", Kind: domain.KindStructure, Title: "domain は infra に依存しない",
		Layers: map[string]string{"domain": "src/domain"}, Allowed: "from-diagram", Forbidden: []string{"domain -> infra"},
		States: []string{"Default", "Empty"}, Strings: []string{"支払一覧"},
	}
	if _, err := r.Restate(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	p := readFile(t, prompt)
	for _, want := range []string{"domain -> infra", "src/domain", "from-diagram", "Empty", "支払一覧"} {
		if !strings.Contains(p, want) {
			t.Fatalf("prompt に %q が無い:\n%s", want, p)
		}
	}
}

func TestCommand_RestateEmptyIsError(t *testing.T) {
	script, _ := fakeCommand(t, "  \n")
	if _, err := (Command{Command: script}).Restate(context.Background(), roundClaim()); err == nil {
		t.Fatal("空の応答は error")
	}
}

func TestCommand_Compare(t *testing.T) {
	script, prompt := fakeCommand(t, "```json\n{\"match\": false, \"differences\": [\"仕様は負数を 0 にするが claim に無い\"]}\n```")
	r := Command{Command: script}
	got, err := r.Compare(context.Background(), "再生成した文", "元の仕様の文")
	if err != nil {
		t.Fatal(err)
	}
	if got.Match || len(got.Differences) != 1 {
		t.Fatalf("got %+v", got)
	}
	p := readFile(t, prompt)
	if !strings.Contains(p, "再生成した文") || !strings.Contains(p, "元の仕様の文") || strings.Contains(p, "{{") {
		t.Fatalf("prompt に両方の文が埋め込まれていない:\n%s", p)
	}
}

func TestCommand_CompareBrokenIsError(t *testing.T) {
	script, _ := fakeCommand(t, "たぶん一致しています")
	if got, err := (Command{Command: script}).Compare(context.Background(), "a", "b"); err == nil {
		t.Fatalf("解釈できない応答は error: %+v", got)
	}
}

func TestCommand_FailureIncludesStderr(t *testing.T) {
	r := Command{Command: "echo 'rate limited' >&2; exit 3"}
	_, err := r.Restate(context.Background(), roundClaim())
	if err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("失敗時は stderr を含める: %v", err)
	}
}

func TestCommand_Timeout(t *testing.T) {
	r := Command{Command: "sleep 5", Timeout: 100 * time.Millisecond}
	start := time.Now()
	if _, err := r.Compare(context.Background(), "a", "b"); err == nil {
		t.Fatal("タイムアウトは error")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("タイムアウトで打ち切られていない")
	}
}
