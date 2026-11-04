package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// docs/spec/harness.md「リテラル漏れ検知」の表
func TestLeakBinding(t *testing.T) {
	cases := []struct{ in, want string }{
		{`{"code": "if x == 1234.56 { return 1234 }", "values": [1234.56, 1234]}`, `[1234.56, 1234]`},
		{`{"code": "return math.Floor(x)", "values": [1234.56, 1234]}`, `[]`},
		{`{"code": "return 0", "values": [0, 1]}`, `[]`},
		{`{"code": "msg := \"円未満は切り捨て\"", "values": ["円未満は切り捨て", "ok"]}`, `["円未満は切り捨て"]`},
	}
	for _, c := range cases {
		var out bytes.Buffer
		if err := funcs.run("leak", strings.NewReader(`{"in": `+c.in+`}`), &out); err != nil {
			t.Fatal(err)
		}
		var got struct{ Out any }
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		var want any
		_ = json.Unmarshal([]byte(c.want), &want)
		g, _ := json.Marshal(got.Out)
		w, _ := json.Marshal(want)
		if string(g) != string(w) {
			t.Fatalf("in=%s: got %s, want %s", c.in, g, w)
		}
	}
}
