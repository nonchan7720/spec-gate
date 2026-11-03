package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/services"
)

func sampleReport() services.CheckReport {
	return services.CheckReport{
		Claims: domain.Results{
			{ID: "A-1", Kind: domain.KindExample, Status: domain.StatusPass, Title: "ok", Spec: "docs/spec/a.md:3"},
			{ID: "A-2", Kind: domain.KindExample, Status: domain.StatusFail, Title: "ng", Detail: "期待=1 実際=2", Spec: "docs/spec/a.md:9"},
			{ID: "U-1", Kind: domain.KindUI, Status: domain.StatusUnverified, Title: "画面"},
		},
		Plans: []services.PlanReport{{Path: "plans/p.md", Title: "p", Items: domain.Results{
			{ID: "p.md", Title: "未凍結", Status: domain.StatusUnapproved},
		}}},
		Skipped: 2,
	}
}

func TestRenderCheck_JSON(t *testing.T) {
	var out bytes.Buffer
	code, err := renderCheck(&out, sampleReport(), checkOutput{JSON: true})
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 {
		t.Fatalf("--json でも blocking があれば exit 1: %d", code)
	}
	var got struct {
		Claims []map[string]any `json:"claims"`
		Plans  []struct {
			Path  string           `json:"path"`
			Title string           `json:"title"`
			Items []map[string]any `json:"items"`
		} `json:"plans"`
		Counts  map[string]int `json:"counts"`
		Skipped int            `json:"skipped"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("出力は JSON 1 つ: %v\n%s", err, out.String())
	}
	if len(got.Claims) != 3 {
		t.Fatalf("pass も含めて全 claim を出す: %+v", got.Claims)
	}
	a2 := got.Claims[1]
	for k, v := range map[string]string{"id": "A-2", "kind": "example", "status": "fail", "title": "ng", "detail": "期待=1 実際=2", "spec": "docs/spec/a.md:9"} {
		if a2[k] != v {
			t.Errorf("claims[1].%s = %v, want %q", k, a2[k], v)
		}
	}
	if len(got.Plans) != 1 || got.Plans[0].Path != "plans/p.md" || got.Plans[0].Items[0]["status"] != "unapproved" {
		t.Fatalf("プランも出す: %+v", got.Plans)
	}
	want := map[string]int{"total": 4, "pass": 1, "fail": 1, "stale": 0, "unapproved": 1, "unverified": 1}
	for k, v := range want {
		if got.Counts[k] != v {
			t.Errorf("counts.%s = %d, want %d（テキストの集計と同じくプラン項目も含む）", k, got.Counts[k], v)
		}
	}
	if got.Skipped != 2 {
		t.Errorf("skipped = %d", got.Skipped)
	}
}

func TestRenderCheck_JSONEmptyIsArrays(t *testing.T) {
	var out bytes.Buffer
	code, err := renderCheck(&out, services.CheckReport{}, checkOutput{JSON: true})
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	s := out.String()
	if !strings.Contains(s, `"claims":[]`) || !strings.Contains(s, `"plans":[]`) {
		t.Fatalf("空でも null ではなく [] を出す（CI の jq で扱いやすく）: %s", s)
	}
}

func TestRenderCheck_TextExitCode(t *testing.T) {
	var out bytes.Buffer
	code, err := renderCheck(&out, sampleReport(), checkOutput{})
	if err != nil || code != 1 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if !strings.Contains(out.String(), "1 pass, 1 fail, 0 stale, 1 unapproved, 1 unverified") {
		t.Fatalf("テキストは従来どおり: %s", out.String())
	}
	rep := services.CheckReport{Claims: domain.Results{{ID: "A", Status: domain.StatusUnverified}}}
	if code, _ := renderCheck(&out, rep, checkOutput{JSON: true}); code != 0 {
		t.Fatalf("unverified だけなら exit 0: %d", code)
	}
}

func TestWriteMetrics(t *testing.T) {
	m := services.ComputeMetrics(sampleReport().Claims)

	var text bytes.Buffer
	if err := writeMetrics(&text, m, false); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"3 total", "example", "66.7%", "ui", "33.3%", "unverified", "1 / 3"} {
		if !strings.Contains(text.String(), want) {
			t.Errorf("テキストに %q が無い:\n%s", want, text.String())
		}
	}

	var js bytes.Buffer
	if err := writeMetrics(&js, m, true); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Total  int `json:"total"`
		ByKind []struct {
			Kind  string  `json:"kind"`
			Count int     `json:"count"`
			Ratio float64 `json:"ratio"`
		} `json:"by_kind"`
		Unverified     int     `json:"unverified"`
		UnverifiedRate float64 `json:"unverified_rate"`
	}
	if err := json.Unmarshal(js.Bytes(), &got); err != nil {
		t.Fatalf("%v\n%s", err, js.String())
	}
	if got.Total != 3 || got.Unverified != 1 || got.UnverifiedRate != 1.0/3 || len(got.ByKind) != 4 || got.ByKind[0].Kind != "example" || got.ByKind[0].Count != 2 {
		t.Fatalf("JSON: %+v", got)
	}
}
