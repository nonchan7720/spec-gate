package domain

import "testing"

func TestUnverifiedRate(t *testing.T) {
	cases := []struct {
		name string
		in   []Status
		want float64
	}{
		{"5 件中 2 件", []Status{StatusPass, StatusPass, StatusUnverified, StatusUnverified, StatusFail}, 0.4},
		{"claim が 0 件なら 0", nil, 0},
		{"すべて unverified", []Status{StatusUnverified}, 1},
		{"unverified なし", []Status{StatusPass, StatusStale}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := UnverifiedRate(c.in); got != c.want {
				t.Fatalf("UnverifiedRate(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestResults_Statuses(t *testing.T) {
	rs := Results{{ID: "A", Status: StatusPass}, {ID: "B", Status: StatusUnverified}}
	got := rs.Statuses()
	if len(got) != 2 || got[0] != StatusPass || got[1] != StatusUnverified {
		t.Fatalf("Statuses は順に status を返す: %v", got)
	}
}
