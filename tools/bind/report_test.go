package main

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// bindRow は docs/spec/harness.md の表の 1 行。in / out は表に書かれた JSON そのもの。
type bindRow struct{ in, out string }

// assertRows は binding を表の各行で呼び、出力が JSON として等しいかを確かめる（check と同じ比べ方）。
func assertRows(t *testing.T, name string, rows []bindRow) {
	t.Helper()
	for _, row := range rows {
		var out bytes.Buffer
		if err := funcs.run(name, strings.NewReader(`{"in": `+row.in+`}`), &out); err != nil {
			t.Errorf("%s %s: %v", name, row.in, err)
			continue
		}
		var got, want any
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("%s: 出力が JSON でない: %q", name, out.String())
		}
		if err := json.Unmarshal([]byte(`{"out": `+row.out+`}`), &want); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s %s = %s, want {\"out\": %s}", name, row.in, strings.TrimSpace(out.String()), row.out)
		}
	}
}

// harness.md「CI 用の出力と指標」の表
func TestBinding_UnverifiedRate(t *testing.T) {
	assertRows(t, "unverified-rate", []bindRow{
		{`["pass", "pass", "unverified", "unverified", "fail"]`, `0.4`},
		{`[]`, `0`},
		{`["unverified"]`, `1`},
	})
}

// harness.md「役割別レポート」の表
func TestBinding_RoleFilter(t *testing.T) {
	assertRows(t, "role-filter", []bindRow{
		{`{"role": "engineer", "results": [{"id": "A", "kind": "example", "status": "fail"}, {"id": "B", "kind": "example", "status": "pass"}]}`, `["A"]`},
		{`{"role": "designer", "results": [{"id": "U", "kind": "ui", "status": "unverified"}, {"id": "E", "kind": "example", "status": "unverified"}]}`, `["U"]`},
		{`{"role": "pm", "results": [{"id": "S", "kind": "example", "status": "stale"}, {"id": "N", "kind": "manual", "status": "unapproved"}, {"id": "F", "kind": "example", "status": "fail"}]}`, `["S", "N"]`},
		{`{"role": "engineer", "results": []}`, `[]`},
	})
}

func TestBinding_RoleFilterRejectsUnknownRole(t *testing.T) {
	var out bytes.Buffer
	if err := funcs.run("role-filter", strings.NewReader(`{"in": {"role": "qa", "results": []}}`), &out); err == nil {
		t.Fatalf("知らない役割はエラー: %q", out.String())
	}
}

func TestBinding_UnverifiedRateRejectsNonArray(t *testing.T) {
	var out bytes.Buffer
	if err := funcs.run("unverified-rate", strings.NewReader(`{"in": "pass"}`), &out); err == nil {
		t.Fatalf("配列でなければエラー: %q", out.String())
	}
}
