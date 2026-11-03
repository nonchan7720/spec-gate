package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/tracelog"
	"github.com/nonchan7720/spec-gate/pkg/services"
)

type fixedNow string

func (c fixedNow) Now() string { return string(c) }

type zeroLines struct{}

func (zeroLines) ChangedLines() (int, error) { return 0, nil }

type brokenLog struct{}

func (brokenLog) Append([]domain.TraceEntry) error  { return errors.New("read-only file system") }
func (brokenLog) All() ([]domain.TraceEntry, error) { return nil, errors.New("read-only file system") }

func TestRecordHookTrace_WritesClaimsOnly(t *testing.T) {
	root := t.TempDir()
	tr := &services.TraceService{Log: tracelog.New(root), Lines: zeroLines{}, Clock: fixedNow("2026-11-04T00:00:00Z")}
	recordHookTrace(tr, roleReport())
	b, err := os.ReadFile(filepath.Join(root, ".spec-gate", "trace.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != len(roleReport().Claims) {
		t.Fatalf("claim ごとに 1 行（プラン項目は claim でないので記録しない）: %d 行\n%s", len(lines), b)
	}
	if lines[1] != `{"time":"2026-11-04T00:00:00Z","id":"EX-FAIL","status":"fail","lines":0}` {
		t.Fatalf("時刻・claim id・status・変更行数: %s", lines[1])
	}
}

func TestRecordHookTrace_NeverFailsTheHook(t *testing.T) {
	tr := &services.TraceService{Log: brokenLog{}, Lines: zeroLines{}, Clock: fixedNow("t")}
	recordHookTrace(tr, roleReport()) // panic せず、何も返さない
	recordHookTrace(nil, roleReport())
	if code, _ := hookResult(services.CheckReport{Claims: domain.Results{{ID: "A", Status: domain.StatusPass}}}); code != 0 {
		t.Fatalf("記録の失敗で hook の結果は変わらない: %d", code)
	}
}

func sampleTraces() []services.ClaimTrace {
	return []services.ClaimTrace{
		{ID: "ROUND-001", Runs: 3, Last: domain.StatusPass, Pass: &domain.TraceSummary{FailsBeforePass: 2, LinesAtPass: 120}},
		{ID: "UI-001", Runs: 2, Last: domain.StatusFail},
	}
}

func TestWriteTrace_Text(t *testing.T) {
	var out bytes.Buffer
	if err := writeTrace(&out, sampleTraces(), false); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{"ROUND-001", "120", "UI-001", "pass していない"} {
		if !strings.Contains(s, want) {
			t.Errorf("%q が無い:\n%s", want, s)
		}
	}
	var empty bytes.Buffer
	if err := writeTrace(&empty, nil, false); err != nil || !strings.Contains(empty.String(), "記録なし") {
		t.Fatalf("記録が無いことを出す: %q %v", empty.String(), err)
	}
}

func TestWriteTrace_JSON(t *testing.T) {
	var out bytes.Buffer
	if err := writeTrace(&out, sampleTraces(), true); err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if len(got) != 2 || got[0]["id"] != "ROUND-001" || got[0]["runs"] != float64(3) || got[0]["last"] != "pass" {
		t.Fatalf("JSON: %+v", got)
	}
	pass, ok := got[0]["pass"].(map[string]any)
	if !ok || pass["fails_before_pass"] != float64(2) || pass["lines_at_pass"] != float64(120) {
		t.Fatalf("pass は binding と同じ形: %+v", got[0])
	}
	if got[1]["pass"] != nil {
		t.Fatalf("pass していなければ null: %+v", got[1])
	}
	var empty bytes.Buffer
	if err := writeTrace(&empty, nil, true); err != nil || strings.TrimSpace(empty.String()) != "[]" {
		t.Fatalf("空なら []: %q", empty.String())
	}
}
