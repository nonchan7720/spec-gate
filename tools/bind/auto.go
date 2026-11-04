package main

import "encoding/json"

// auto-approval.md（承認の自動化）の binding。
//
// まだ実装していない（plans/2026-11-auto-approval.md で実装する）。名前だけ先に登録し、出力は null にする。
// 未登録だと抽出の試し実行が error になり承認できないが、登録して null を返すと fail（期待と違う）になり、
// 実装の前に claim を承認できる（先にテストを赤で置くのと同じ）。
func init() {
	for _, name := range []string{"auto-policy", "auto-run-commands", "auto-eligible", "auto-evidence", "auto-breaker", "guard"} {
		register(name, notImplemented)
	}
}

func notImplemented(json.RawMessage) (any, error) { return nil, nil }
