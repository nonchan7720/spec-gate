package main

import (
	"bytes"
	"strings"
	"testing"
)

// 仕様（docs/spec/harness.md「ランダム入力による検証」）の入出力表。
func TestRelation_SpecTable(t *testing.T) {
	cases := []struct{ in, want string }{
		{`{"relation": "f(x + 1) == f(x) + 1", "x": 1.5, "values": {"f(x)": 1, "f(x + 1)": 2}}`, `{"out":true}`},
		{`{"relation": "f(x + 1) == f(x) + 1", "x": 1.5, "values": {"f(x)": 1, "f(x + 1)": 3}}`, `{"out":false}`},
	}
	for _, tc := range cases {
		var out bytes.Buffer
		if err := funcs.run("relation", strings.NewReader(`{"in": `+tc.in+`}`), &out); err != nil {
			t.Fatalf("%s: %v", tc.in, err)
		}
		if got := strings.TrimSpace(out.String()); got != tc.want {
			t.Errorf("%s: got %s want %s", tc.in, got, tc.want)
		}
	}
}

func TestRelation_Errors(t *testing.T) {
	for _, in := range []string{
		`{"relation": "f(x * 2) == 1", "x": 1, "values": {"f(x)": 1}}`, // 値の無い呼び出し
		`{"relation": "f(x) +", "x": 1, "values": {"f(x)": 1}}`,        // 式として不正
		`{"x": 1, "values": {}}`,                                       // relation が無い
		`"f(x)"`,                                                       // オブジェクトでない
	} {
		var out bytes.Buffer
		if err := funcs.run("relation", strings.NewReader(`{"in": `+in+`}`), &out); err == nil {
			t.Errorf("%s: エラーになるべき: %s", in, out.String())
		}
	}
}
