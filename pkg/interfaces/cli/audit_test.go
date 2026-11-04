package cli

import (
	"testing"
	"time"
)

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	cases := map[string]string{
		"":                          "",
		"24h":                       "2026-11-03T12:00:00Z",
		"90m":                       "2026-11-04T10:30:00Z",
		"2026-11-04T09:00:00+09:00": "2026-11-04T00:00:00Z",
	}
	for in, want := range cases {
		if got, err := parseSince(in, now); err != nil || got != want {
			t.Errorf("parseSince(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"yesterday", "-1h"} {
		if _, err := parseSince(bad, now); err == nil {
			t.Errorf("parseSince(%q) はエラー", bad)
		}
	}
}
