package main

import "testing"

// harness.md「manual と例のある claim の同定」の表
func TestBinding_ConsensusKinds(t *testing.T) {
	ex := `{"kind": "example", "title": "変更されたファイルが触るファイルそのものなら対象", "in": "a.ts", "out": true}`
	man := `{"kind": "manual", "title": "変更されたファイルが、触るファイルそのものなら、その claim は対象"}`
	assertRows(t, "consensus-kinds", []bindRow{
		{`[[` + ex + `], [` + man + `]]`, `[{"kind": "example", "hits": 2}]`},
		{`[[` + man + `], [` + ex + `]]`, `[{"kind": "example", "hits": 2}]`},
		{`[[` + ex + `, ` + man + `]]`, `[{"kind": "example", "hits": 1}, {"kind": "manual", "hits": 1}]`},
		{`[[{"kind": "example", "title": "円未満は切り捨てる", "in": 1.5, "out": 1}], [{"kind": "manual", "title": "締め処理は月末日に行う"}]]`, `[{"kind": "example", "hits": 1}, {"kind": "manual", "hits": 1}]`},
	})
}

// harness.md「抽出結果の試し実行」の表
func TestBinding_ExampleValueAndDryRunSummary(t *testing.T) {
	assertRows(t, "example-value", []bindRow{
		{`"{\"touches\": [\"a.ts\"], \"changed\": \"a.ts\"}"`, `{"touches": ["a.ts"], "changed": "a.ts"}`},
		{`" [1, 2] "`, `[1, 2]`},
		{`"123"`, `"123"`},
		{`"{壊れている"`, `"{壊れている"`},
		{`"abc"`, `"abc"`},
		{`{"a": "[1]"}`, `{"a": "[1]"}`},
		{`1.5`, `1.5`},
	})
	assertRows(t, "dry-run-summary", []bindRow{
		{`{"total": 3, "mismatch": 0, "error": ""}`, `"pass"`},
		{`{"total": 3, "mismatch": 1, "error": ""}`, `"fail: 3 件中 1 件が期待と違う"`},
		{`{"total": 3, "mismatch": 0, "error": "binding が失敗 (exit status 1)\n詳細"}`, `"error: binding が失敗 (exit status 1)"`},
		{`{"total": 0, "mismatch": 0, "error": ""}`, `"none"`},
	})
}

// harness.md「まとめ承認」「manual の手がかり」の表
func TestBinding_ReviewOrderBulkEligibleManualRate(t *testing.T) {
	assertRows(t, "review-order", []bindRow{
		{`[{"name": "a.yaml", "kind": "manual", "confidence": "3/3", "dry_run": ""}, {"name": "b.yaml", "kind": "example", "confidence": "1/3", "dry_run": "pass"}, {"name": "c.yaml", "kind": "example", "confidence": "3/3", "dry_run": "pass"}]`, `["c.yaml", "b.yaml", "a.yaml"]`},
		{`[{"name": "a.yaml", "kind": "example", "confidence": "3/3", "dry_run": "error: x"}, {"name": "b.yaml", "kind": "example", "confidence": "3/3", "dry_run": "fail: 2 件中 1 件が期待と違う"}, {"name": "c.yaml", "kind": "ui", "confidence": "3/3", "dry_run": ""}]`, `["c.yaml", "b.yaml", "a.yaml"]`},
		{`[{"name": "b.yaml", "kind": "manual", "confidence": "2/3", "dry_run": ""}, {"name": "a.yaml", "kind": "manual", "confidence": "2/3", "dry_run": ""}]`, `["a.yaml", "b.yaml"]`},
	})
	assertRows(t, "bulk-eligible", []bindRow{
		{`{"kind": "example", "confidence": "3/3", "dry_run": "pass", "min_agree": 0}`, `true`},
		{`{"kind": "example", "confidence": "3/3", "dry_run": "fail: 3 件中 1 件が期待と違う", "min_agree": 0}`, `false`},
		{`{"kind": "manual", "confidence": "3/3", "dry_run": "", "min_agree": 0}`, `true`},
		{`{"kind": "manual", "confidence": "2/3", "dry_run": "", "min_agree": 0}`, `false`},
		{`{"kind": "manual", "confidence": "2/3", "dry_run": "", "min_agree": 2}`, `true`},
		{`{"kind": "example", "confidence": "", "dry_run": "pass", "min_agree": 0}`, `false`},
	})
	assertRows(t, "manual-rate", []bindRow{
		{`["example", "manual", "manual", "ui"]`, `0.5`},
		{`["example"]`, `0`},
		{`[]`, `0`},
	})
}
