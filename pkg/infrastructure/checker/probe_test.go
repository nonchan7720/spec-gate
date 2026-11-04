package checker

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// scriptedBinding は入力ごとに決めた出力・エラーを返す。
type scriptedBinding struct {
	out string
	err error
}

func (s scriptedBinding) run(context.Context, string, []byte) (string, error) { return s.out, s.err }

func probeClaim(examples ...domain.Example) domain.Claim {
	return domain.Claim{ID: "P-1", Kind: domain.KindExample, Title: "t", Binding: "cmd",
		Source: domain.Source{Type: domain.SourceMarkdown, Ref: "spec.md#x"}, Examples: examples}
}

func TestProbe(t *testing.T) {
	ctx := context.Background()
	floor := &fakeBinding{fn: func(x float64) any { return float64(int(x)) }}
	got := (&ExampleChecker{runner: floor}).Probe(ctx, probeClaim(domain.Example{In: 1.5, Out: 1}, domain.Example{In: 2.5, Out: 3}))
	if got.Summary() != "fail: 2 件中 1 件が期待と違う" {
		t.Errorf("期待と違う例は fail として数える: %q", got.Summary())
	}
	if len(floor.calls) != 2 {
		t.Errorf("examples だけを流す: %v", floor.calls)
	}

	got = (&ExampleChecker{runner: floor}).Probe(ctx, probeClaim(domain.Example{In: 1.5, Out: 1}))
	if got.Summary() != "pass" {
		t.Errorf("すべて期待どおりなら pass: %q", got.Summary())
	}

	broken := scriptedBinding{out: "touched: json: cannot unmarshal string\nexit status 1", err: errors.New("exit status 1")}
	got = (&ExampleChecker{runner: broken}).Probe(ctx, probeClaim(domain.Example{In: 1, Out: 1}, domain.Example{In: 2, Out: 2}))
	if got.State() != domain.DryRunError || !strings.Contains(got.Summary(), "cannot unmarshal string") {
		t.Errorf("binding が失敗したら error（最後の行を理由に出す）: %q", got.Summary())
	}

	notJSON := scriptedBinding{out: "hello"}
	if got := (&ExampleChecker{runner: notJSON}).Probe(ctx, probeClaim(domain.Example{In: 1, Out: 1})); got.State() != domain.DryRunError {
		t.Errorf("出力が JSON でなければ error: %q", got.Summary())
	}

	manual := domain.Claim{ID: "M-1", Kind: domain.KindManual, Title: "t"}
	if got := (&ExampleChecker{runner: broken}).Probe(ctx, manual); got.State() != domain.DryRunNone {
		t.Errorf("example でなければ試さない: %q", got.Summary())
	}
}

func TestShellRunner_BindingReadsOnlyStdout(t *testing.T) {
	r := ShellRunner{Dir: t.TempDir()}
	out, err := r.run(context.Background(), `echo 'npm warn exec tsx' >&2; printf '{"out":1}'; echo 'npm notice' >&2`, []byte("{}"))
	if err != nil || out != `{"out":1}` {
		t.Fatalf("成功した binding は stdout だけを返す（stderr の警告を混ぜない）: %q, %v", out, err)
	}
	out, err = r.run(context.Background(), `echo partial; echo 'boom' >&2; exit 3`, nil)
	if err == nil || !strings.Contains(out, "partial") || !strings.Contains(out, "boom") {
		t.Fatalf("失敗したら理由のため stdout と stderr の両方を返す: %q, %v", out, err)
	}
	out, _ = r.Run(context.Background(), `echo a; echo b >&2`)
	if !strings.Contains(out, "a") || !strings.Contains(out, "b") {
		t.Fatalf("verify 行（Run）は従来どおり両方を返す: %q", out)
	}
}
