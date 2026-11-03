package domain

import (
	"reflect"
	"testing"
)

func hist(pairs ...any) []TraceEntry {
	var out []TraceEntry
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, TraceEntry{Status: pairs[i].(Status), Lines: pairs[i+1].(int)})
	}
	return out
}

func TestSummarizeTrace(t *testing.T) {
	cases := []struct {
		name string
		in   []TraceEntry
		want *TraceSummary
	}{
		// harness.md「通り方のログ」の表
		{"2 回 fail してから pass", hist(StatusFail, 10, StatusFail, 40, StatusPass, 120), &TraceSummary{FailsBeforePass: 2, LinesAtPass: 120}},
		{"最初から pass", hist(StatusPass, 5), &TraceSummary{FailsBeforePass: 0, LinesAtPass: 5}},
		{"pass していない", hist(StatusFail, 3), nil},
		// 最後に pass になったときを見る
		{"pass が続いたら最初の pass の時点", hist(StatusFail, 1, StatusPass, 7, StatusPass, 9), &TraceSummary{FailsBeforePass: 1, LinesAtPass: 7}},
		{"一度落ちてまた pass したら後の方", hist(StatusFail, 1, StatusPass, 2, StatusFail, 30, StatusFail, 50, StatusFail, 60, StatusPass, 300), &TraceSummary{FailsBeforePass: 3, LinesAtPass: 300}},
		{"pass の後に fail で終わっても最後の pass", hist(StatusFail, 1, StatusPass, 2, StatusFail, 3), &TraceSummary{FailsBeforePass: 1, LinesAtPass: 2}},
		{"連続 fail は fail 以外で途切れる", hist(StatusFail, 1, StatusStale, 2, StatusFail, 3, StatusPass, 4), &TraceSummary{FailsBeforePass: 1, LinesAtPass: 4}},
		{"空", nil, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SummarizeTrace(c.in); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("SummarizeTrace = %+v, want %+v", got, c.want)
			}
		})
	}
}
