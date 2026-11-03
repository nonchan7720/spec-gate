package textutil

import "testing"

func TestTail(t *testing.T) {
	cases := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"短ければそのまま（前後の空白は除く）", "\na\nb\n", 5, "a\nb"},
		{"ちょうど n 行", "a\nb\nc", 3, "a\nb\nc"},
		{"超えたら末尾 n 行と省略数", "a\nb\nc\nd", 2, "… (2 lines omitted)\nc\nd"},
		{"空文字", "", 3, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Tail(c.in, c.n); got != c.want {
				t.Fatalf("Tail(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
			}
		})
	}
}
