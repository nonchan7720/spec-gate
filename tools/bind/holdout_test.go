package main

import (
	"bytes"
	"strings"
	"testing"
)

// docs/spec/harness.md「holdout examples」の表
func TestHoldoutSummaryBinding(t *testing.T) {
	cases := []struct{ in, want string }{
		{`[{"pass": true}, {"pass": true}]`, `{"out":"holdout 2 件すべて一致"}`},
		{`[{"pass": true}, {"pass": false, "in": 1.5, "out": 1, "got": 2}]`, `{"out":"holdout 2 件中 1 件が不一致（値は表示しない）"}`},
		{`[]`, `{"out":"holdout なし"}`},
	}
	for _, c := range cases {
		var out bytes.Buffer
		if err := funcs.run("holdout-summary", strings.NewReader(`{"in": `+c.in+`}`), &out); err != nil {
			t.Fatal(err)
		}
		if got := strings.TrimSpace(out.String()); got != c.want {
			t.Fatalf("in=%s: got %s, want %s", c.in, got, c.want)
		}
	}
}
