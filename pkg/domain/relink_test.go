package domain

import "testing"

func TestSuggestHeading(t *testing.T) {
	cases := []struct {
		name     string
		missing  string
		headings []string
		want     string
		ok       bool
	}{
		// 仕様の表（docs/spec/harness.md「見出し変更時の再リンク提案」）
		{"1 文字足しただけの見出しを提案する", "端数処理", []string{"端数の処理", "マイナス金額"}, "端数の処理", true},
		{"遠い見出ししか無ければ提案しない", "端数処理", []string{"背景", "変更履歴"}, "", false},

		// 閾値: 距離が見出しの長さ（文字数）の半分以下
		{"距離がちょうど半分なら提案する", "abcd", []string{"abxy"}, "abxy", true},
		{"距離が半分を超えたら提案しない", "abcd", []string{"axyz"}, "", false},
		{"奇数長は切り捨てた半分まで（5 文字なら 2）", "abcde", []string{"abcxy"}, "abcxy", true},
		{"奇数長で 3 は半分を超える", "abcde", []string{"abxyz"}, "", false},
		{"長さはバイトでなく文字で数える", "締め処理", []string{"締め日"}, "締め日", true},

		// 同点は文書の順で先のもの
		{"同点なら先に出てくる見出し", "abcd", []string{"abcx", "abcy"}, "abcx", true},
		{"より近い見出しが後ろにあればそちら", "abcd", []string{"abxy", "abcx"}, "abcx", true},
		{"見出しが無ければ提案しない", "端数処理", nil, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := SuggestHeading(tc.missing, tc.headings)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("SuggestHeading(%q, %q) = (%q, %v), want (%q, %v)", tc.missing, tc.headings, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestEditDistance(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"kitten", "sitting", 3},
		{"端数処理", "端数の処理", 1},
		{"端数処理", "背景", 4},
	}
	for _, tc := range cases {
		if got := editDistance(tc.a, tc.b); got != tc.want {
			t.Errorf("editDistance(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
