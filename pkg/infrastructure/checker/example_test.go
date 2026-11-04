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
