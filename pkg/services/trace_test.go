package services

import (
	"errors"
	"reflect"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

type fakeTraceLog struct {
	entries   []domain.TraceEntry
	appendErr error
}

func (f *fakeTraceLog) Append(es []domain.TraceEntry) error {
	if f.appendErr != nil {
		return f.appendErr
	}
	f.entries = append(f.entries, es...)
	return nil
}
func (f *fakeTraceLog) All() ([]domain.TraceEntry, error) { return f.entries, nil }

type fakeLines struct {
	n   int
	err error
}

func (f fakeLines) ChangedLines() (int, error) { return f.n, f.err }

func TestTraceRecord_OneEntryPerClaim(t *testing.T) {
	log := &fakeTraceLog{}
	s := &TraceService{Log: log, Lines: fakeLines{n: 42}, Clock: fixedClock("2026-11-04T01:02:03Z")}
	err := s.Record(domain.Results{
		{ID: "A", Status: domain.StatusFail},
		{ID: "B", Status: domain.StatusPass},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.TraceEntry{
		{Time: "2026-11-04T01:02:03Z", ID: "A", Status: domain.StatusFail, Lines: 42},
		{Time: "2026-11-04T01:02:03Z", ID: "B", Status: domain.StatusPass, Lines: 42},
	}
	if !reflect.DeepEqual(log.entries, want) {
		t.Fatalf("claim ごとに時刻・id・status・変更行数を記録: %+v", log.entries)
	}
}

func TestTraceRecord_LineCountErrorIsZero(t *testing.T) {
	log := &fakeTraceLog{}
	s := &TraceService{Log: log, Lines: fakeLines{err: errors.New("git が無い")}, Clock: fixedClock("t")}
	if err := s.Record(domain.Results{{ID: "A", Status: domain.StatusPass}}); err != nil {
		t.Fatalf("行数が取れなくても記録は続ける: %v", err)
	}
	if len(log.entries) != 1 || log.entries[0].Lines != 0 {
		t.Fatalf("行数が取れなければ 0: %+v", log.entries)
	}
}

func TestTraceRecord_AppendErrorIsReturned(t *testing.T) {
	s := &TraceService{Log: &fakeTraceLog{appendErr: errors.New("disk full")}, Lines: fakeLines{}, Clock: fixedClock("t")}
	if err := s.Record(domain.Results{{ID: "A", Status: domain.StatusPass}}); err == nil {
		t.Fatal("書けなかったことは呼び出し側に返す（hook を落とすかは呼び出し側が決める）")
	}
}

func TestTraceSummary_PerClaim(t *testing.T) {
	log := &fakeTraceLog{entries: []domain.TraceEntry{
		{ID: "B", Status: domain.StatusFail, Lines: 3},
		{ID: "A", Status: domain.StatusFail, Lines: 10},
		{ID: "A", Status: domain.StatusFail, Lines: 40},
		{ID: "B", Status: domain.StatusFail, Lines: 9},
		{ID: "A", Status: domain.StatusPass, Lines: 120},
	}}
	got, err := (&TraceService{Log: log}).Summary()
	if err != nil {
		t.Fatal(err)
	}
	want := []ClaimTrace{
		{ID: "A", Runs: 3, Last: domain.StatusPass, Pass: &domain.TraceSummary{FailsBeforePass: 2, LinesAtPass: 120}},
		{ID: "B", Runs: 2, Last: domain.StatusFail},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("claim ごとに id 順でまとめる: %+v", got)
	}
}
