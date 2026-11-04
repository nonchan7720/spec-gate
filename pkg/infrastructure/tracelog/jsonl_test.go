package tracelog

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

func TestAppendAndAll(t *testing.T) {
	root := t.TempDir()
	l := New(root)
	if got, err := l.All(); err != nil || len(got) != 0 {
		t.Fatalf("ファイルが無ければ空: %v %v", got, err)
	}
	first := []domain.TraceEntry{{Time: "t1", ID: "A", Status: domain.StatusFail, Lines: 10}, {Time: "t1", ID: "B", Status: domain.StatusPass, Lines: 10}}
	second := []domain.TraceEntry{{Time: "t2", ID: "A", Status: domain.StatusPass, Lines: 40}}
	if err := l.Append(first); err != nil {
		t.Fatal(err)
	}
	if err := l.Append(second); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, ".spec-gate", "trace.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	if len(lines) != 3 || lines[0] != `{"time":"t1","id":"A","status":"fail","lines":10}` {
		t.Fatalf(".spec-gate/trace.jsonl に 1 判定 1 行で追記する: %q", string(b))
	}
	got, err := l.All()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, append(first, second...)) {
		t.Fatalf("古い順に読める: %+v", got)
	}
}

func TestAll_SkipsBrokenLines(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, ".spec-gate", "trace.jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"time":"t1","id":"A","status":"fail","lines":1}` + "\n" + `{"time":"t2","id":` + "\n\n" + `{"time":"t3","id":"A","status":"pass","lines":2}` + "\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := New(root).All()
	if err != nil || len(got) != 2 || got[1].Time != "t3" {
		t.Fatalf("途中で切れた行（書き込み中の中断など）は飛ばして読む: %+v %v", got, err)
	}
}

func TestAppend_EmptyIsNoop(t *testing.T) {
	root := t.TempDir()
	if err := New(root).Append(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".spec-gate", "trace.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("記録が無ければファイルを作らない: %v", err)
	}
}
