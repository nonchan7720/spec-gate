package main

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// docs/spec/harness.md「mutation score」の表。stdout の JSON まで一致させる。
func TestMutateBinding_SpecTable(t *testing.T) {
	cases := []struct{ in, want string }{
		{`"a + b"`, `{"out":["a - b"]}`},
		{`"a * b"`, `{"out":["a / b"]}`},
		{`"x > 0"`, `{"out":["x >= 0","x < 0"]}`},
		{`"x == y"`, `{"out":["x != y"]}`},
		{`"return 100"`, `{"out":["return 101"]}`},
		{`"return x"`, `{"out":[]}`},
	}
	for _, c := range cases {
		var out bytes.Buffer
		if err := funcs.run("mutate", strings.NewReader(`{"in": `+c.in+`}`), &out); err != nil {
			t.Fatalf("%s: %v", c.in, err)
		}
		// check は JSON として比べる（encoding/json は < > を < > と書くが値は同じ）
		var got, want any
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("%s: stdout が JSON でない: %q", c.in, out.String())
		}
		_ = json.Unmarshal([]byte(c.want), &want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("mutate %s = %s, want %s", c.in, strings.TrimSpace(out.String()), c.want)
		}
	}
}

func TestMutateBinding_RejectsNonString(t *testing.T) {
	var out bytes.Buffer
	if err := funcs.run("mutate", strings.NewReader(`{"in": 1}`), &out); err == nil {
		t.Fatal("入力はコード 1 行（文字列）")
	}
}
