package adapter

import (
	"context"
	"reflect"
	"testing"
)

func edges(t *testing.T, body string) []string {
	t.Helper()
	root := t.TempDir()
	writeSpec(t, root, "a.mmd", body)
	e, err := NewMermaid(root).Edges("a.mmd")
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestMermaidEdges_Forms(t *testing.T) {
	got := edges(t, `%% comment --> ignored
graph TD
  app[app: 画面] --> domain[domain: 業務]
  app -.-> infra
  infra ==> domain
  a -- text --> b
  c -->|label| d
`)
	want := []string{"a -> b", "app -> domain", "app -> infra", "c -> d", "infra -> domain"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("edges = %v, want %v", got, want)
	}
}

func TestMermaidEdges_Chain(t *testing.T) {
	// a --> b --> c は 2 本のエッジ。2 本目を読まないと、そこが変わっても stale にならない
	got := edges(t, "graph LR\n  a[A] --> b(B) --> c{C}\n")
	want := []string{"a -> b", "b -> c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("edges = %v, want %v", got, want)
	}
}

func TestMermaidSnapshot_LabelsDoNotStaleButEdgesDo(t *testing.T) {
	root := t.TempDir()
	m := NewMermaid(root)
	hash := func(body string) string {
		writeSpec(t, root, "a.mmd", body)
		s, err := m.Snapshot(context.Background(), "a.mmd")
		if err != nil || s.Skipped != "" {
			t.Fatalf("%v %q", err, s.Skipped)
		}
		return s.Hash
	}
	base := hash("graph TD\n  a[A] --> b[B]\n")
	if hash("graph LR\n  a[別のラベル] --> b\n") != base {
		t.Fatal("ラベル・向きだけの変更では stale にしない")
	}
	if hash("graph TD\n  a --> b --> c\n") == base {
		t.Fatal("エッジが増えたら stale")
	}
}

func TestMermaidSnapshot_NoEdgesIsSkipped(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, "a.mmd", "sequenceDiagram\n  A->>B: hi\n")
	s, err := NewMermaid(root).Snapshot(context.Background(), "a.mmd")
	if err != nil || s.Skipped == "" {
		t.Fatalf("エッジが読めない図は skipped: %+v %v", s, err)
	}
}
