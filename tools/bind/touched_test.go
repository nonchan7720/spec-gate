package main

import (
	"bytes"
	"strings"
	"testing"
)

// 仕様の表（docs/spec/harness.md「変更検知の精度」）をそのまま流す。
func TestTouched_SpecTable(t *testing.T) {
	cases := []struct{ in, want string }{
		{`{"touches": ["src/payroll/round.ts"], "changed": "src/payroll/round.ts"}`, `{"out":true}`},
		{`{"touches": ["src/payroll/round.ts"], "changed": "src/payroll/util.ts"}`, `{"out":true}`},
		{`{"touches": ["src/payroll/round.ts"], "changed": "src/payroll/README.md"}`, `{"out":false}`},
		{`{"touches": ["src/app"], "changed": "src/app/handler/x.ts"}`, `{"out":true}`},
		{`{"touches": ["src/payroll/round.ts"], "changed": "src/ui/List.tsx"}`, `{"out":false}`},
		{`{"touches": ["tools/bind", "pkg/domain"], "changed": "pkg/domain/relink.go"}`, `{"out":true}`},
	}
	for _, tc := range cases {
		var out bytes.Buffer
		if err := funcs.run("touched", strings.NewReader(`{"in": `+tc.in+`}`), &out); err != nil {
			t.Fatal(err)
		}
		if got := strings.TrimSpace(out.String()); got != tc.want {
			t.Errorf("touched %s = %s, want %s", tc.in, got, tc.want)
		}
	}
}

func TestTouched_BadInput(t *testing.T) {
	var out bytes.Buffer
	if err := funcs.run("touched", strings.NewReader(`{"in": "x"}`), &out); err == nil {
		t.Fatal("形の違う入力はエラーにする")
	}
}
