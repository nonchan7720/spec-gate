package domain

import (
	"errors"
	"hash/fnv"
	"math"
	"strings"
	"testing"
)

func fuzzClaim(f *Fuzz, relations []string) Claim {
	c := exampleClaim("FZ-1", nil, []string{"isInteger(output)"})
	c.Fuzz = f
	c.Relations = relations
	return c
}

func validFuzz() *Fuzz {
	return &Fuzz{Runs: 10, Input: FuzzInput{Type: "number", Min: new(0.0), Max: new(100.0)}}
}

func TestFuzz_SameSeedSameInputs(t *testing.T) {
	f := Fuzz{Runs: 20, Seed: new(int64(42)), Input: FuzzInput{Type: "number", Min: new(-5.0), Max: new(5.0)}}
	a, b := f.Inputs("X-1"), f.Inputs("Y-2")
	if len(a) != 20 || canon(a) != canon(b) {
		t.Fatalf("seed が同じなら claim ID に関係なく同じ入力列: %v / %v", a, b)
	}
	g := f
	g.Seed = new(int64(43))
	if canon(g.Inputs("X-1")) == canon(a) {
		t.Fatal("seed が違えば入力列も変わる")
	}
}

func TestFuzz_DefaultSeedFromClaimID(t *testing.T) {
	f := Fuzz{Runs: 10, Input: FuzzInput{Type: "number", Min: new(0.0), Max: new(1.0)}}
	h := fnv.New64a()
	_, _ = h.Write([]byte("A-1"))
	if f.EffectiveSeed("A-1") != int64(h.Sum64()) {
		t.Fatal("seed 省略時は claim ID の FNV-1a（64 bit）")
	}
	first := canon(f.Inputs("A-1"))
	if canon(f.Inputs("A-1")) != first {
		t.Fatal("seed 省略時も毎回同じ入力列")
	}
	if f.EffectiveSeed("A-1") == f.EffectiveSeed("A-2") {
		t.Fatal("claim ID が違えば既定の seed も違う")
	}
	// 既定の seed を明示しても同じ入力列になる（反例の再現に使える）
	g := f
	g.Seed = new(f.EffectiveSeed("A-1"))
	if canon(g.Inputs("other")) != canon(f.Inputs("A-1")) {
		t.Fatal("報告した seed を書けば同じ入力列を再現できる")
	}
}

func TestFuzz_InputsInRange(t *testing.T) {
	num := Fuzz{Runs: 500, Input: FuzzInput{Type: "number", Min: new(-2.5), Max: new(3.5)}}
	for _, v := range num.Inputs("N") {
		x := v.(float64)
		if x < -2.5 || x > 3.5 {
			t.Fatalf("number が範囲外: %v", x)
		}
	}
	integer := Fuzz{Runs: 500, Input: FuzzInput{Type: "integer", Min: new(-3.0), Max: new(3.0)}}
	seen := map[int64]bool{}
	for _, v := range integer.Inputs("I") {
		x := v.(int64)
		if x < -3 || x > 3 {
			t.Fatalf("integer が範囲外: %v", x)
		}
		seen[x] = true
	}
	if !seen[-3] || !seen[3] {
		t.Fatalf("integer は min / max を含む閉区間: %v", seen)
	}
	single := Fuzz{Runs: 3, Input: FuzzInput{Type: "integer", Min: new(7.0), Max: new(7.0)}}
	if canon(single.Inputs("S")) != "[7,7,7]" {
		t.Fatalf("min == max なら常にその値: %v", single.Inputs("S"))
	}
}

func TestValidate_Fuzz(t *testing.T) {
	if err := fuzzClaim(validFuzz(), []string{"f(x) >= 0"}).Validate(); err != nil {
		t.Fatalf("正しい fuzz は通る: %v", err)
	}
	cases := map[string]func(f *Fuzz){
		"runs":         func(f *Fuzz) { f.Runs = 0 },
		"runs 上限":      func(f *Fuzz) { f.Runs = MaxFuzzRuns + 1 },
		"input.type":   func(f *Fuzz) { f.Input.Type = "string" },
		"input.min":    func(f *Fuzz) { f.Input.Min = nil },
		"input.max":    func(f *Fuzz) { f.Input.Max = nil },
		"min > max":    func(f *Fuzz) { f.Input.Min = new(10.0); f.Input.Max = new(1.0) },
		"integer の端数":  func(f *Fuzz) { f.Input.Type = "integer"; f.Input.Min = new(0.5) },
		"有限でない":        func(f *Fuzz) { f.Input.Max = new(math.Inf(1)) },
		"integer の大きさ": func(f *Fuzz) { f.Input.Type = "integer"; f.Input.Max = new(1e300) },
	}
	for name, mutate := range cases {
		f := validFuzz()
		mutate(f)
		if err := fuzzClaim(f, nil).Validate(); err == nil || !strings.Contains(err.Error(), "fuzz") {
			t.Errorf("%s: 不正な fuzz を弾くべき: %v", name, err)
		}
	}
}

func TestValidate_RelationsNeedFuzz(t *testing.T) {
	if err := fuzzClaim(nil, []string{"f(x) >= 0"}).Validate(); err == nil {
		t.Fatal("relations は fuzz の入力で評価するので fuzz が必要")
	}
	c := fuzzClaim(validFuzz(), nil)
	c.Properties = nil
	if err := c.Validate(); err == nil {
		t.Fatal("fuzz には評価する properties か relations が必要")
	}
	c.Relations = []string{"f(x + 1) == f(x) + 1"}
	if err := c.Validate(); err != nil {
		t.Fatalf("examples が無くても fuzz + relations なら通る: %v", err)
	}
	ui := Claim{ID: "U-1", Kind: KindUI, Title: "t", Source: Source{Type: SourceFigma, Ref: "figma://a/b"}, Strings: []string{"x"}, Fuzz: validFuzz()}
	if err := ui.Validate(); err == nil {
		t.Fatal("fuzz は kind: example だけ")
	}
}

func TestEvalRelation_CallsFPerArgument(t *testing.T) {
	var args []any
	f := func(v any) (any, error) {
		args = append(args, v)
		return math.Floor(v.(float64)), nil
	}
	ok, err := EvalRelation("f(x + 1) == f(x) + 1", 1.5, f)
	if err != nil || !ok {
		t.Fatalf("floor は f(x+1) == f(x)+1 を満たす: %v %v", ok, err)
	}
	if canon(args) != "[2.5,1.5]" {
		t.Fatalf("f には式の引数を評価した値が渡る: %v", args)
	}
	ok, err = EvalRelation("f(x * 2) == f(x) * 2", 1.5, f)
	if err != nil || ok {
		t.Fatalf("floor(3) != floor(1.5)*2 なので false: %v %v", ok, err)
	}
}

func TestEvalRelation_IntegerInputAndError(t *testing.T) {
	ok, err := EvalRelation("f(x) == x", int64(3), func(v any) (any, error) { return v, nil })
	if err != nil || !ok {
		t.Fatalf("integer の入力も数値として比べられる: %v %v", ok, err)
	}
	boom := errors.New("boom")
	if _, err := EvalRelation("f(x) == 1", 1.0, func(any) (any, error) { return nil, boom }); !errors.Is(err, boom) {
		t.Fatalf("f の失敗はそのまま返す: %v", err)
	}
}

func TestEvalRelationValues(t *testing.T) {
	values := map[string]any{"f(x)": 1.0, "f(x + 1)": 2.0}
	ok, err := EvalRelationValues("f(x + 1) == f(x) + 1", 1.5, values)
	if err != nil || !ok {
		t.Fatalf("true になるはず: %v %v", ok, err)
	}
	values["f(x + 1)"] = 3.0
	ok, err = EvalRelationValues("f(x + 1) == f(x) + 1", 1.5, values)
	if err != nil || ok {
		t.Fatalf("false になるはず: %v %v", ok, err)
	}
	// 空白の違いは同じ呼び出しとして扱う
	ok, err = EvalRelationValues("f(x+1) == f(x) + 1", 1.5, map[string]any{"f(x)": 1, "f( x + 1 )": 2})
	if err != nil || !ok {
		t.Fatalf("空白違いのキーも引ける: %v %v", ok, err)
	}
	if _, err := EvalRelationValues("f(x * 2) == 0", 1.5, map[string]any{"f(x)": 1}); err == nil {
		t.Fatal("値の無い f(...) はエラー")
	}
	if _, err := EvalRelationValues("f(x) == 0", 1.5, map[string]any{"g(x)": 1}); err == nil {
		t.Fatal("f(...) でないキーはエラー")
	}
}

func TestLint_InvalidRelation(t *testing.T) {
	for _, rel := range []string{"f(x) +", "g(x) == 1", "f(x, 1) == 1", "f(input) == output", "f(x) + 1"} {
		c := fuzzClaim(validFuzz(), []string{rel})
		issues := Lint([]Claim{c})
		if len(issues) != 1 || !strings.Contains(issues[0].Message, "relation") {
			t.Errorf("%q を弾くべき: %+v", rel, issues)
		}
	}
	if issues := Lint([]Claim{fuzzClaim(validFuzz(), []string{"f(x + 1) == f(x) + 1"})}); len(issues) != 0 {
		t.Fatalf("正しい relation は通る: %+v", issues)
	}
}
