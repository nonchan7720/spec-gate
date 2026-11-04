package domain

import (
	"reflect"
	"testing"
)

// 仕様（docs/spec/harness.md「mutation score」）の表の各行。順序まで一致させる。
func TestMutateLine_SpecTable(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"a + b", []string{"a - b"}},
		{"a * b", []string{"a / b"}},
		{"x > 0", []string{"x >= 0", "x < 0"}},
		{"x == y", []string{"x != y"}},
		{"return 100", []string{"return 101"}},
		{"return x", []string{}},
	}
	for _, c := range cases {
		got := MutateLine(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("MutateLine(%q) = %#v, want %#v", c.in, got, c.want)
		}
	}
}

// 仕様の変異一覧（これで全部）のうち表に無い演算子。
func TestMutateLine_ExhaustiveOperatorList(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"a - b", []string{"a + b"}},
		{"a / b", []string{"a * b"}},
		{"x < y", []string{"x <= y", "x > y"}},
		{"x >= y", []string{"x > y"}},
		{"x <= y", []string{"x < y"}},
		{"x != y", []string{"x == y"}},
		// 一覧に無い演算子は変えない
		{"x === y", []string{}},
		{"x !== y", []string{}},
		{"x % y", []string{}},
	}
	for _, c := range cases {
		got := MutateLine(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("MutateLine(%q) = %#v, want %#v", c.in, got, c.want)
		}
	}
}

// 1 回に 1 か所だけ変える。複数か所あれば行の左から順に並べる。
func TestMutateLine_OnePlaceAtATimeInOrder(t *testing.T) {
	got := MutateLine("if a + 100 > b {")
	want := []string{"if a - 100 > b {", "if a + 101 > b {", "if a + 100 >= b {", "if a + 100 < b {"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestMutateLine_SkipsStringsAndComments(t *testing.T) {
	cases := []string{
		`msg := "a + b > 100"`,
		`msg := 'a * b'`,
		"msg := `x == 100`",
		`// a + b == 100`,
		`/* x > 0 */`,
		`s := "\"a + b\""`,
	}
	for _, in := range cases {
		if got := MutateLine(in); len(got) != 0 {
			t.Errorf("MutateLine(%q) は文字列・コメントを変えない: %#v", in, got)
		}
	}
	// 文字列・コメントの外側は変える
	got := MutateLine(`x := a + "b + c" // d + e`)
	want := []string{`x := a - "b + c" // d + e`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

// コンパイルが通らないだけの変異（＝ verify が強くなくても落ちる）を作らない。
func TestMutateLine_SkipsNonBinaryAndCompoundOperators(t *testing.T) {
	cases := []string{
		"p := *q",
		"func f(p *T) {}",
		"var s []*T",
		"x := -y",
		"i++",
		"i--",
		"x += y",
		"x -= y",
		"v := <-ch",
		"f := (a) => a",
		"a && b",
		"x := 0",
		"x := 0x10",
		"x := 1.5",
		"x := 1_000",
		"x := v2",
		"mode := 0755",
		"x := 1e9",
	}
	for _, in := range cases {
		if got := MutateLine(in); len(got) != 0 {
			t.Errorf("MutateLine(%q) は変えない: %#v", in, got)
		}
	}
}

// 整数リテラルは 2 以上だけ n+1。0 と 1 は変えない。
func TestMutateLine_IntegerLiteralsFromTwo(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"return 0", []string{}},
		{"return 1", []string{}},
		{"return 2", []string{"return 3"}},
		{"return 99", []string{"return 100"}},
		{"xs[1:]", []string{}},
	}
	for _, c := range cases {
		got := MutateLine(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("MutateLine(%q) = %#v, want %#v", c.in, got, c.want)
		}
	}
}

func TestMutateLine_NegativeLiteral(t *testing.T) {
	got := MutateLine("x := -5")
	want := []string{"x := -6"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

// ファイル全体では複数行にまたがるコメント・文字列も飛ばす。
func TestMutateSource_MultilineCommentsAndRawStrings(t *testing.T) {
	src := "/*\na + b\n*/\ns := `\nx > 0\n`\nreturn a * 2\n"
	got := MutateSource(src)
	want := []Mutant{
		{Line: 7, Before: "return a * 2", After: "return a / 2"},
		{Line: 7, Before: "return a * 2", After: "return a * 3"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestMutant_Apply(t *testing.T) {
	src := "a\nreturn a * 2\nb\n"
	m := MutateSource(src)[0]
	if got := m.Apply(src); got != "a\nreturn a / 2\nb\n" {
		t.Fatalf("Apply = %q", got)
	}
}

func TestMutationScore(t *testing.T) {
	cases := []struct {
		killed, total int
		want          float64
	}{
		{4, 5, 0.8},
		{0, 3, 0},
		{3, 3, 1},
		{0, 0, 0},
	}
	for _, c := range cases {
		r := MutationResult{Killed: c.killed, Total: c.total}
		if got := r.Score(); got != c.want {
			t.Errorf("Score(%d/%d) = %v, want %v", c.killed, c.total, got, c.want)
		}
	}
}

func TestPickMutants_SpreadsEvenlyUpToMax(t *testing.T) {
	var all []int
	for i := range 10 {
		all = append(all, i)
	}
	if got := PickEvenly(all, 0); len(got) != 10 {
		t.Fatalf("max 0 は無制限: %v", got)
	}
	if got := PickEvenly(all, 20); len(got) != 10 {
		t.Fatalf("max が件数以上なら全部: %v", got)
	}
	got := PickEvenly(all, 5)
	if !reflect.DeepEqual(got, []int{0, 2, 4, 6, 8}) {
		t.Fatalf("均等に間引く: %v", got)
	}
}
