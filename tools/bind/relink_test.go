package main

import (
	"bytes"
	"strings"
	"testing"
)

// 仕様の表（docs/spec/harness.md「見出し変更時の再リンク提案」）をそのまま流す。
func TestRelink_SpecTable(t *testing.T) {
	cases := []struct{ in, want string }{
		{`{"missing": "端数処理", "headings": ["端数の処理", "マイナス金額"]}`, `{"out":"端数の処理"}`},
		{`{"missing": "端数処理", "headings": ["背景", "変更履歴"]}`, `{"out":null}`},
	}
	for _, tc := range cases {
		var out bytes.Buffer
		if err := funcs.run("relink", strings.NewReader(`{"in": `+tc.in+`}`), &out); err != nil {
			t.Fatal(err)
		}
		if got := strings.TrimSpace(out.String()); got != tc.want {
			t.Errorf("relink %s = %s, want %s", tc.in, got, tc.want)
		}
	}
}

func TestRelink_BadInput(t *testing.T) {
	var out bytes.Buffer
	if err := funcs.run("relink", strings.NewReader(`{"in": 1}`), &out); err == nil {
		t.Fatal("形の違う入力はエラーにする")
	}
}
