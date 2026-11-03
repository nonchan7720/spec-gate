package checker

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// fakeBinding は binding コマンドの代わりに Go の関数を呼び、呼ばれた入力を記録する。
type fakeBinding struct {
	fn    func(x float64) any
	calls []float64
}

func (f *fakeBinding) run(_ context.Context, _ string, stdin []byte) (string, error) {
	var req struct {
		In float64 `json:"in"`
	}
	if err := json.Unmarshal(stdin, &req); err != nil {
		return "", err
	}
	f.calls = append(f.calls, req.In)
	b, _ := json.Marshal(map[string]any{"out": f.fn(req.In)})
	return "log line\n" + string(b), nil
}

func fuzzExampleClaim(props, relations []string) domain.Claim {
	lo, hi := -100.0, 100.0
	return domain.Claim{
		ID: "FZ-1", Kind: domain.KindExample, Title: "t",
		Source:     domain.Source{Type: domain.SourceMarkdown, Ref: "spec.md#x"},
		Binding:    "cmd",
		Examples:   []domain.Example{{In: 1.5, Out: 1}},
		Properties: props,
		Fuzz:       &domain.Fuzz{Runs: 50, Input: domain.FuzzInput{Type: "number", Min: &lo, Max: &hi}},
		Relations:  relations,
	}
}

func TestExample_FuzzPassesForCorrectImplementation(t *testing.T) {
	b := &fakeBinding{fn: func(x float64) any { return math.Floor(x) }}
	c := fuzzExampleClaim([]string{"isInteger(output)", "output <= input"}, []string{"f(x + 1) == f(x) + 1"})
	res := (&ExampleChecker{runner: b}).Check(context.Background(), c, domain.CheckOptions{})
	if res.Status != domain.StatusPass {
		t.Fatalf("正しい実装は pass: %s", res.Detail)
	}
	// examples 1 回 + 生成入力ごとに f(x) と f(x + 1) の 2 回（f(x) は使い回す）
	if len(b.calls) != 1+50*2 {
		t.Fatalf("binding の呼び出し回数 = %d", len(b.calls))
	}
}

func TestExample_FuzzFindsCounterexampleForProperty(t *testing.T) {
	// examples の 1.5 → 1 だけ満たす実装（それ以外は入力をそのまま返す）
	b := &fakeBinding{fn: func(x float64) any {
		if x == 1.5 {
			return 1
		}
		return x
	}}
	c := fuzzExampleClaim([]string{"isInteger(output)"}, nil)
	res := (&ExampleChecker{runner: b}).Check(context.Background(), c, domain.CheckOptions{})
	if res.Status != domain.StatusFail {
		t.Fatal("examples だけ通す実装は fuzz で fail")
	}
	seed := c.Fuzz.EffectiveSeed(c.ID)
	first := c.Fuzz.Inputs(c.ID)[0]
	for _, want := range []string{"反例", fmt.Sprintf("seed=%d", seed), fmt.Sprintf("x=%v", first), "isInteger(output)"} {
		if !strings.Contains(res.Detail, want) {
			t.Errorf("detail に %q を含むべき: %s", want, res.Detail)
		}
	}
	// 最初の反例で打ち切る
	if len(b.calls) != 2 {
		t.Fatalf("最初の反例で止める: calls=%v", b.calls)
	}
}

func TestExample_FuzzFindsCounterexampleForRelation(t *testing.T) {
	// 整数を返すが、-1 < x < 0 で f(x + 1) == f(x) + 1 を満たさない（0 方向への切り捨て）
	b := &fakeBinding{fn: func(x float64) any { return math.Trunc(x) }}
	c := fuzzExampleClaim([]string{"isInteger(output)"}, []string{"f(x + 1) == f(x) + 1"})
	lo, hi := -0.99, -0.01
	c.Fuzz.Input.Min, c.Fuzz.Input.Max = &lo, &hi
	res := (&ExampleChecker{runner: b}).Check(context.Background(), c, domain.CheckOptions{})
	if res.Status != domain.StatusFail || !strings.Contains(res.Detail, "relation") || !strings.Contains(res.Detail, "反例") {
		t.Fatalf("relation の反例で fail: %v %s", res.Status, res.Detail)
	}
}

func TestExample_FuzzReusesResultsForSameInput(t *testing.T) {
	b := &fakeBinding{fn: func(x float64) any { return x }}
	c := fuzzExampleClaim(nil, []string{"f(x) == f(x)", "f(x * 1) == x"})
	lo := 3.0
	c.Examples = nil
	c.Fuzz = &domain.Fuzz{Runs: 5, Input: domain.FuzzInput{Type: "integer", Min: &lo, Max: &lo}}
	res := (&ExampleChecker{runner: b}).Check(context.Background(), c, domain.CheckOptions{})
	if res.Status != domain.StatusPass {
		t.Fatalf("pass のはず: %s", res.Detail)
	}
	if len(b.calls) != 1 {
		t.Fatalf("同じ入力の binding は 1 回だけ呼ぶ: %v", b.calls)
	}
}

func TestExample_FuzzSkippedWhenExamplesFail(t *testing.T) {
	b := &fakeBinding{fn: func(x float64) any { return 99 }}
	c := fuzzExampleClaim([]string{"isInteger(output)"}, nil)
	res := (&ExampleChecker{runner: b}).Check(context.Background(), c, domain.CheckOptions{})
	if res.Status != domain.StatusFail || len(b.calls) != 1 {
		t.Fatalf("examples が不一致なら fuzz は回さない: %v calls=%d", res.Status, len(b.calls))
	}
}

// identity は {"in": X} を {"out": X} にして返す binding（恒等関数）。
const identity = `sed 's/"in"/"out"/'`

func exampleClaim(binding string, exs ...domain.Example) domain.Claim {
	return domain.Claim{ID: "ID-001", Kind: domain.KindExample, Title: "t", Binding: binding, Examples: exs}
}

func runExample(t *testing.T, c domain.Claim, holdout []domain.Example) domain.Result {
	t.Helper()
	chk := NewExample(ShellRunner{Dir: t.TempDir()})
	return chk.Check(context.Background(), c, domain.CheckOptions{Holdout: holdout})
}

func assertNoValues(t *testing.T, detail string, values ...string) {
	t.Helper()
	for _, v := range values {
		if strings.Contains(detail, v) {
			t.Fatalf("holdout の値 %q を出してはならない:\n%s", v, detail)
		}
	}
}

func TestExample_HoldoutAllPass(t *testing.T) {
	res := runExample(t, exampleClaim(identity, domain.Example{In: 1, Out: 1}),
		[]domain.Example{{In: 7777.25, Out: 7777.25}, {In: "abcd", Out: "abcd"}})
	if res.Status != domain.StatusPass {
		t.Fatalf("holdout がすべて一致なら pass: %+v", res)
	}
	assertNoValues(t, res.Detail, "7777", "abcd")
}

func TestExample_HoldoutMismatchFailsWithCountsOnly(t *testing.T) {
	res := runExample(t, exampleClaim(identity, domain.Example{In: 1, Out: 1}),
		[]domain.Example{{In: 7777.25, Out: 7777.25}, {In: 31337, Out: 4242}})
	if res.Status != domain.StatusFail {
		t.Fatalf("holdout が 1 件でも不一致なら fail: %+v", res)
	}
	if !strings.Contains(res.Detail, "holdout 2 件中 1 件が不一致（値は表示しない）") {
		t.Fatalf("件数だけを出す: %q", res.Detail)
	}
	assertNoValues(t, res.Detail, "7777", "31337", "4242")
}

func TestExample_HoldoutPropertyViolationIsCountedWithoutValues(t *testing.T) {
	c := exampleClaim(identity, domain.Example{In: 1, Out: 1})
	c.Properties = []string{"output < 5000"}
	res := runExample(t, c, []domain.Example{{In: 7777.25, Out: 7777.25}})
	if res.Status != domain.StatusFail || !strings.Contains(res.Detail, "holdout 1 件中 1 件が不一致") {
		t.Fatalf("property を満たさない holdout も不一致: %+v", res)
	}
	assertNoValues(t, res.Detail, "7777")
}

func TestExample_HoldoutBindingFailureHidesOutput(t *testing.T) {
	// binding が入力をそのまま stderr に吐いて落ちる（エラー出力に holdout の値が混ざる）
	c := exampleClaim(`cat >&2; exit 1`)
	c.Properties = []string{"true"}
	res := runExample(t, c, []domain.Example{{In: 7777.25, Out: 1}, {In: 31337, Out: 2}})
	if res.Status != domain.StatusFail || !strings.Contains(res.Detail, "holdout 2 件中 2 件が不一致") {
		t.Fatalf("binding が落ちたら残りも不一致として数える: %+v", res)
	}
	assertNoValues(t, res.Detail, "7777", "31337")
}

func TestExample_HoldoutNonJSONOutputHidden(t *testing.T) {
	c := exampleClaim(`cat`) // {"in": ...} をそのまま返す（out が無い）→ 出力を表示すると値が漏れる
	c.Properties = []string{"true"}
	res := runExample(t, c, []domain.Example{{In: 7777.25, Out: 1}})
	if res.Status != domain.StatusFail {
		t.Fatalf("out が一致しなければ fail: %+v", res)
	}
	assertNoValues(t, res.Detail, "7777")
}

func TestExample_ExamplesStillReportValues(t *testing.T) {
	res := runExample(t, exampleClaim(identity, domain.Example{In: 5, Out: 6}), nil)
	if res.Status != domain.StatusFail || !strings.Contains(res.Detail, "期待=6") {
		t.Fatalf("公開の examples は従来どおり値を出す: %+v", res)
	}
	if strings.Contains(res.Detail, "holdout") {
		t.Fatalf("holdout が無ければ holdout の行は出さない: %q", res.Detail)
	}
}
