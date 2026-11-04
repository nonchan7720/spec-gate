package main

import (
	"bytes"
	"strings"
	"testing"
)

// harness.md「通り方のログ」の表
func TestBinding_TraceSummary(t *testing.T) {
	assertRows(t, "trace-summary", []bindRow{
		{`[{"status": "fail", "lines": 10}, {"status": "fail", "lines": 40}, {"status": "pass", "lines": 120}]`, `{"fails_before_pass": 2, "lines_at_pass": 120}`},
		{`[{"status": "pass", "lines": 5}]`, `{"fails_before_pass": 0, "lines_at_pass": 5}`},
		{`[{"status": "fail", "lines": 3}]`, `null`},
		{`[]`, `null`},
	})
}

func TestBinding_TraceSummaryRejectsNonArray(t *testing.T) {
	var out bytes.Buffer
	if err := funcs.run("trace-summary", strings.NewReader(`{"in": {"status": "pass"}}`), &out); err == nil {
		t.Fatalf("配列でなければエラー: %q", out.String())
	}
}
