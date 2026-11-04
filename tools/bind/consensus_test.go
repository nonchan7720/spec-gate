package main

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// 仕様「合議の同定」の binding 表。
func TestConsensusHits_SpecTable(t *testing.T) {
	cases := []struct{ in, want string }{
		{`[["提案だけを行い、claim は書き換えない（claim の修正は人間が行う）"], ["再リンクの提案は表示だけで、claim ファイルは書き換えない（claim の修正は人間が行う）"], ["見出しの提案だけを行い、claim は書き換えない（claim の修正は人間が行う）"]]`,
			`[3]`},
		{`[["check --role engineer は fail だけを出す"], ["check --role pm は stale と unapproved を出す"]]`,
			`[1, 1]`},
		{`[["算術の変異は + → -、- → +、* → /、/ → * の 4 種で全部とする"], ["比較の変異は > → >= と <、< → <= と >、>= → >、<= → <、== → !=、!= → == で全部とする"]]`,
			`[1, 1]`},
		{`[["変更行数は git diff --numstat HEAD の追加行と削除行の合計とし、git リポジトリでなければ 0 とする", "trace に記録する変更行数は git diff --numstat HEAD の追加行と削除行の合計とし、git リポジトリでなければ 0 とする"], ["変更行数は git diff --numstat HEAD の追加行と削除行の合計とする。git リポジトリでなければ 0"]]`,
			`[2, 1]`},
		{`[["管理者だけが削除できる【要確認: 権限表が無い】"], ["管理者だけが、削除できる。"]]`,
			`[2]`},
	}
	for _, c := range cases {
		var out bytes.Buffer
		if err := funcs.run("consensus-hits", strings.NewReader(`{"in": `+c.in+`}`), &out); err != nil {
			t.Fatal(err)
		}
		var got struct{ Out any }
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		var want any
		if err := json.Unmarshal([]byte(c.want), &want); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got.Out, want) {
			t.Errorf("in %s: out = %v, want %v", c.in, got.Out, want)
		}
	}
}
