package domain

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"reflect"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

// MaxFuzzRuns は fuzz.runs の上限。binding はプロセス起動なので呼び出し回数を抑える。
const MaxFuzzRuns = 10000

// maxExactInteger は float64 で整数を正確に表せる上限（2^53）。
const maxExactInteger = 1 << 53

// Fuzz はランダム入力による検証の設定。
type Fuzz struct {
	Runs int `yaml:"runs"`
	// Seed は乱数の種。省略時は claim ID から決める（EffectiveSeed）。
	Seed  *int64    `yaml:"seed,omitempty"`
	Input FuzzInput `yaml:"input"`
}

// FuzzInput は生成する入力の型と範囲（min / max を含む閉区間）。
type FuzzInput struct {
	Type string   `yaml:"type"` // number | integer
	Min  *float64 `yaml:"min"`
	Max  *float64 `yaml:"max"`
}

// validate は fuzz 設定の構造を確かめる。
func (f Fuzz) validate() error {
	switch {
	case f.Runs <= 0 || f.Runs > MaxFuzzRuns:
		return fmt.Errorf("fuzz.runs は 1〜%d: %d", MaxFuzzRuns, f.Runs)
	case f.Input.Type != "number" && f.Input.Type != "integer":
		return fmt.Errorf("fuzz.input.type は number か integer: %q", f.Input.Type)
	case f.Input.Min == nil || f.Input.Max == nil:
		return fmt.Errorf("fuzz.input.min と fuzz.input.max が必要")
	}
	lo, hi := *f.Input.Min, *f.Input.Max
	switch {
	case math.IsNaN(lo) || math.IsInf(lo, 0) || math.IsNaN(hi) || math.IsInf(hi, 0):
		return fmt.Errorf("fuzz.input.min / max は有限の数")
	case lo > hi:
		return fmt.Errorf("fuzz.input.min (%v) が max (%v) より大きい", lo, hi)
	}
	if f.Input.Type == "integer" {
		if lo != math.Trunc(lo) || hi != math.Trunc(hi) {
			return fmt.Errorf("fuzz.input.type: integer の min / max は整数: %v / %v", lo, hi)
		}
		if math.Abs(lo) > maxExactInteger/2 || math.Abs(hi) > maxExactInteger/2 {
			return fmt.Errorf("fuzz.input.type: integer の min / max は ±2^52 以内: %v / %v", lo, hi)
		}
	}
	return nil
}

// EffectiveSeed は使う乱数の種。seed を省略したら claim ID の FNV-1a ハッシュ。
func (f Fuzz) EffectiveSeed(claimID string) int64 {
	if f.Seed != nil {
		return *f.Seed
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(claimID))
	return int64(h.Sum64())
}

// Inputs は runs 個の入力を種から決定的に生成する。
// number は float64、integer は int64（JSON でも整数で渡るように）。
func (f Fuzz) Inputs(claimID string) []any {
	r := rand.New(rand.NewPCG(uint64(f.EffectiveSeed(claimID)), 0x5eed))
	lo, hi := *f.Input.Min, *f.Input.Max
	out := make([]any, f.Runs)
	for i := range out {
		if f.Input.Type == "integer" {
			out[i] = int64(lo) + r.Int64N(int64(hi)-int64(lo)+1)
			continue
		}
		v := lo + r.Float64()*(hi-lo)
		out[i] = min(max(v, lo), hi) // 丸め誤差で範囲をはみ出さない
	}
	return out
}

// relationEnv は relation 式の環境。x と isInteger は property と同じ扱い。
func relationEnv(x any) map[string]any {
	return map[string]any{
		"x": normalizeNumber(x),
		"isInteger": func(v any) bool {
			f, ok := toFloat(v)
			return ok && f == math.Trunc(f)
		},
	}
}

// EvalRelation はメタモルフィック関係を評価する。f(arg) は引数を評価した値で call を呼ぶ。
// call の失敗はそのまま返す（errors.Is で取り出せる）。
func EvalRelation(relation string, x any, call func(arg any) (any, error)) (bool, error) {
	v, err := runWithF(relation, x, call, expr.AsBool())
	if err != nil {
		return false, err
	}
	return v.(bool), nil
}

// runWithF は x と f(...) を使う式 src を評価する。f(arg) は call の値を数値の型をそろえて返す。
func runWithF(src string, x any, call func(arg any) (any, error), opts ...expr.Option) (any, error) {
	env := relationEnv(x)
	fn := expr.Function("f", func(params ...any) (any, error) {
		out, err := call(params[0])
		if err != nil {
			return nil, err
		}
		return normalizeNumber(out), nil
	}, new(func(any) any))
	prog, err := expr.Compile(src, append([]expr.Option{expr.Env(env), fn}, opts...)...)
	if err != nil {
		return nil, err
	}
	return expr.Run(prog, env)
}

// CheckRelation は relation が式として正しいか（x と f(...) だけで真偽を返すか）を確かめる。
// f(...) の戻り値は型が決まらないので、AsBool だけでは "f(x) + 1" を通してしまう。
// 式の根が静的に bool（比較・論理演算など）であることを求める。
func CheckRelation(relation string) error {
	prog, err := expr.Compile(relation, expr.Env(relationEnv(0.0)),
		expr.Function("f", func(...any) (any, error) { return nil, nil }, new(func(any) any)),
		expr.AsBool())
	if err != nil {
		return err
	}
	if t := prog.Node().Type(); t == nil || t.Kind() != reflect.Bool {
		return fmt.Errorf("比較や論理演算で真偽を返す式にする（例: f(x + 1) == f(x) + 1）")
	}
	return nil
}

// EvalRelationValues は f(...) の値を外から与えて relation を評価する。
// values のキーは "f(x + 1)" のような呼び出しの式で、その引数を x で評価した値に対応づける。
// 空白の違いは問わない（引数の値で引くため）。
func EvalRelationValues(relation string, x any, values map[string]any) (bool, error) {
	type entry struct {
		arg ast.Node
		out any
	}
	pending := map[string]entry{}
	for key, out := range values {
		tree, err := parser.Parse(key)
		if err != nil {
			return false, fmt.Errorf("values のキー %q が式でない: %w", key, err)
		}
		call, ok := tree.Node.(*ast.CallNode)
		if id, isIdent := callee(call); !ok || !isIdent || id != "f" || len(call.Arguments) != 1 {
			return false, fmt.Errorf("values のキー %q は f(...) の形にする", key)
		}
		pending[key] = entry{call.Arguments[0], out}
	}

	known := map[string]any{} // 引数の値（正規化 JSON）→ f の値
	lookup := func(arg any) (any, error) {
		if out, ok := known[canon(arg)]; ok {
			return out, nil
		}
		return nil, fmt.Errorf("f(%v) の値が values にない", arg)
	}
	// 引数に f を含むキー（f(f(x)) など）もあるので、評価できたものから埋めていく
	for len(pending) > 0 {
		progress := false
		var lastErr error
		for key, e := range pending {
			arg, err := evalArg(e.arg.String(), x, lookup)
			if err != nil {
				lastErr = fmt.Errorf("values のキー %q: %w", key, err)
				continue
			}
			if prev, dup := known[canon(arg)]; dup && canon(prev) != canon(e.out) {
				return false, fmt.Errorf("values のキー %q: 同じ引数 %v に異なる値（%v と %v）", key, arg, prev, e.out)
			}
			known[canon(arg)] = normalizeNumber(e.out)
			delete(pending, key)
			progress = true
		}
		if !progress {
			return false, lastErr
		}
	}
	return EvalRelation(relation, x, lookup)
}

func callee(call *ast.CallNode) (string, bool) {
	if call == nil {
		return "", false
	}
	id, ok := call.Callee.(*ast.IdentifierNode)
	if !ok {
		return "", false
	}
	return id.Value, true
}

// evalArg は f(...) の引数の式を x で評価する。
func evalArg(arg string, x any, call func(any) (any, error)) (any, error) {
	v, err := runWithF(arg, x, call)
	if err != nil {
		return nil, err
	}
	return normalizeNumber(v), nil
}
