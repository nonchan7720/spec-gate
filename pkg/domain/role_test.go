package domain

import (
	"reflect"
	"testing"
)

func ids(rs Results) []string {
	out := []string{}
	for _, r := range rs {
		out = append(out, r.ID)
	}
	return out
}

func TestFilterByRole(t *testing.T) {
	all := Results{
		{ID: "EX-FAIL", Kind: KindExample, Status: StatusFail},
		{ID: "EX-PASS", Kind: KindExample, Status: StatusPass},
		{ID: "EX-UNV", Kind: KindExample, Status: StatusUnverified},
		{ID: "UI-UNV", Kind: KindUI, Status: StatusUnverified},
		{ID: "UI-FAIL", Kind: KindUI, Status: StatusFail},
		{ID: "UI-PASS", Kind: KindUI, Status: StatusPass},
		{ID: "UI-STALE", Kind: KindUI, Status: StatusStale},
		{ID: "STALE", Kind: KindExample, Status: StatusStale},
		{ID: "UNAPPROVED", Kind: KindManual, Status: StatusUnapproved},
	}
	cases := map[Role][]string{
		RoleEngineer: {"EX-FAIL", "UI-FAIL"},              // fail だけ
		RoleDesigner: {"UI-UNV", "UI-FAIL"},               // kind: ui の unverified と fail
		RolePM:       {"UI-STALE", "STALE", "UNAPPROVED"}, // stale と unapproved
	}
	for role, want := range cases {
		if got := ids(FilterByRole(role, all)); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %v, want %v（入力の順を保つ）", role, got, want)
		}
	}
}

func TestParseRole(t *testing.T) {
	for _, s := range []string{"engineer", "designer", "pm"} {
		if r, err := ParseRole(s); err != nil || string(r) != s {
			t.Errorf("ParseRole(%q) = %q, %v", s, r, err)
		}
	}
	if _, err := ParseRole("qa"); err == nil {
		t.Error("知らない役割はエラー（黙って全件や 0 件を出さない）")
	}
}
