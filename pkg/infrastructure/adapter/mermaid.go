package adapter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// Mermaid はフローチャート（graph / flowchart）のエッジ集合を仕様とみなす。
// ノードのラベルや配置が変わってもエッジ集合が同じならハッシュは変わらない。
type Mermaid struct{ root string }

func NewMermaid(root string) *Mermaid { return &Mermaid{root: root} }

func (Mermaid) Type() domain.SourceType { return domain.SourceMermaid }

// `a --> b`, `a -.-> b`, `a ==> b`, `a -- text --> b`, `a -->|text| b` などに対応。
// ノードは `id[label]` / `id(label)` / `id{label}` 形式を許し id だけを取る。
var edgeRe = regexp.MustCompile(`^\s*([A-Za-z0-9_]+)(?:[\[\(\{][^\]\)\}]*[\]\)\}])?\s*(?:-{1,3}[^-=>]*)?(?:-{2,}|={2,}|-\.-?)>\s*(?:\|[^|]*\|)?\s*([A-Za-z0-9_]+)`)

// Edges は "from -> to" の整列済みリストを返す。
func (m *Mermaid) Edges(ref string) ([]string, error) {
	b, err := os.ReadFile(filepath.Join(m.root, ref))
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for line := range strings.SplitSeq(string(b), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "%%") {
			continue
		}
		if mt := edgeRe.FindStringSubmatch(line); mt != nil {
			set[mt[1]+" -> "+mt[2]] = true
		}
	}
	out := make([]string, 0, len(set))
	for e := range set {
		out = append(out, e)
	}
	sort.Strings(out)
	return out, nil
}

func (m *Mermaid) Snapshot(_ context.Context, ref string) (domain.Snapshot, error) {
	edges, err := m.Edges(ref)
	if err != nil {
		return domain.Snapshot{Skipped: fmt.Sprintf("ファイルが読めない: %v", err)}, nil
	}
	if len(edges) == 0 {
		return domain.Snapshot{Skipped: "エッジが 1 つも読み取れない（graph/flowchart のみ対応）"}, nil
	}
	return domain.Snapshot{
		Hash:     sha(strings.Join(edges, "\n")),
		Excerpt:  strings.Join(edges, "\n"),
		Location: ref + ":1",
	}, nil
}

// Units はエッジ 1 本ずつ（coverage 用）。
func (m *Mermaid) Units(_ context.Context, ref string) ([]string, error) {
	edges, err := m.Edges(ref)
	if err != nil {
		return nil, err
	}
	for i := range edges {
		edges[i] = ref + "#" + edges[i]
	}
	return edges, nil
}
