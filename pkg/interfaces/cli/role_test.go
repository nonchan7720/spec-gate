package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/services"
)

func roleReport() services.CheckReport {
	return services.CheckReport{
		Claims: domain.Results{
			{ID: "EX-PASS", Kind: domain.KindExample, Status: domain.StatusPass, Title: "ok", Spec: "docs/spec/a.md:3"},
			{ID: "EX-FAIL", Kind: domain.KindExample, Status: domain.StatusFail, Title: "ng", Detail: "期待=1 実際=2", Spec: "docs/spec/a.md:9"},
			{ID: "EX-NOSPEC", Kind: domain.KindExample, Status: domain.StatusFail, Title: "仕様が読めない", Detail: "仕様を読めない"},
			{ID: "UI-UNV", Kind: domain.KindUI, Status: domain.StatusUnverified, Title: "画面"},
			{ID: "STALE", Kind: domain.KindExample, Status: domain.StatusStale, Title: "古い"},
			{ID: "NEW", Kind: domain.KindManual, Status: domain.StatusUnapproved, Title: "未承認"},
		},
		Plans: []services.PlanReport{{Path: "plans/p.md", Title: "p", Items: domain.Results{
			{ID: "p.md:L4", Title: "項目", Status: domain.StatusFail, Detail: "$ false"},
		}}},
	}
}

func TestRenderCheck_RoleEngineer(t *testing.T) {
	var out bytes.Buffer
	code, err := renderCheck(&out, roleReport(), checkOutput{Role: domain.RoleEngineer})
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 {
		t.Fatalf("--role でも exit code は通常の check と同じ: %d", code)
	}
	s := out.String()
	for _, want := range []string{"EX-FAIL", "[docs/spec/a.md:9]", "期待=1 実際=2", "EX-NOSPEC", "仕様の位置不明", "p.md:L4", "[plans/p.md]"} {
		if !strings.Contains(s, want) {
			t.Errorf("engineer は fail を仕様の位置つきで出す。%q が無い:\n%s", want, s)
		}
	}
	for _, ng := range []string{"EX-PASS", "UI-UNV", "STALE", "NEW"} {
		if strings.Contains(s, ng) {
			t.Errorf("engineer に %s は出さない:\n%s", ng, s)
		}
	}
}

func TestRenderCheck_RoleDesignerAndPM(t *testing.T) {
	cases := map[domain.Role]struct{ want, ng []string }{
		domain.RoleDesigner: {want: []string{"UI-UNV"}, ng: []string{"EX-FAIL", "STALE", "NEW", "p.md:L4"}},
		domain.RolePM:       {want: []string{"STALE", "NEW"}, ng: []string{"EX-FAIL", "UI-UNV", "EX-PASS", "p.md:L4"}},
	}
	for role, c := range cases {
		var out bytes.Buffer
		code, err := renderCheck(&out, roleReport(), checkOutput{Role: role})
		if err != nil || code != 1 {
			t.Fatalf("%s: code=%d err=%v（exit code は通常と同じ）", role, code, err)
		}
		s := out.String()
		for _, w := range c.want {
			if !strings.Contains(s, w) {
				t.Errorf("%s に %s を出す:\n%s", role, w, s)
			}
		}
		for _, n := range c.ng {
			if strings.Contains(s, n) {
				t.Errorf("%s に %s は出さない:\n%s", role, n, s)
			}
		}
	}
}

func TestRenderCheck_RoleNothingToDo(t *testing.T) {
	rep := services.CheckReport{Claims: domain.Results{{ID: "A", Kind: domain.KindExample, Status: domain.StatusPass}}}
	var out bytes.Buffer
	code, _ := renderCheck(&out, rep, checkOutput{Role: domain.RolePM})
	if code != 0 || !strings.Contains(out.String(), "0 件") {
		t.Fatalf("対応すべきものが無いことを出す: code=%d %q", code, out.String())
	}
}

func TestRenderCheck_RoleWithJSON(t *testing.T) {
	var out bytes.Buffer
	code, err := renderCheck(&out, roleReport(), checkOutput{JSON: true, Role: domain.RolePM})
	if err != nil || code != 1 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	var got struct {
		Role   string `json:"role"`
		Claims []struct {
			ID string `json:"id"`
		} `json:"claims"`
		Plans  []any          `json:"plans"`
		Counts map[string]int `json:"counts"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Role != "pm" || len(got.Claims) != 2 || got.Claims[0].ID != "STALE" || got.Claims[1].ID != "NEW" || len(got.Plans) != 0 {
		t.Fatalf("--json --role は役割で絞った結果を出す: %+v", got)
	}
	if got.Counts["total"] != 7 {
		t.Fatalf("counts は絞り込み前の判定全体の集計: %+v", got.Counts)
	}
}
