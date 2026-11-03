package adapter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// Markdown は `path#見出し` で仕様の節を指す。
// 見出し省略時はファイル全体。ハッシュは正規化後の本文から取る。
type Markdown struct{ root string }

func NewMarkdown(root string) *Markdown { return &Markdown{root: root} }

func (Markdown) Type() domain.SourceType { return domain.SourceMarkdown }

type section struct {
	heading string
	level   int
	body    string
	line    int
}

var headingRe = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*#*\s*$`)

// SplitRef は path#anchor を分解する。
func SplitRef(ref string) (file, anchor string) {
	if before, after, ok := strings.Cut(ref, "#"); ok {
		return before, strings.TrimSpace(after)
	}
	return ref, ""
}

func (m *Markdown) sections(file string) ([]section, error) {
	b, err := os.ReadFile(filepath.Join(m.root, file))
	if err != nil {
		return nil, err
	}
	var out []section
	var cur *section
	inFence := false
	for i, raw := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(strings.TrimSpace(raw), "```") {
			inFence = !inFence
		}
		if !inFence {
			if mt := headingRe.FindStringSubmatch(raw); mt != nil {
				if cur != nil {
					out = append(out, *cur)
				}
				cur = &section{heading: mt[2], level: len(mt[1]), line: i + 1}
				continue
			}
		}
		if cur != nil {
			cur.body += raw + "\n"
		}
	}
	if cur != nil {
		out = append(out, *cur)
	}
	return out, nil
}

func (m *Markdown) Snapshot(_ context.Context, ref string) (domain.Snapshot, error) {
	file, anchor := SplitRef(ref)
	if anchor == "" {
		b, err := os.ReadFile(filepath.Join(m.root, file))
		if err != nil {
			return domain.Snapshot{Skipped: fmt.Sprintf("ファイルが読めない: %v", err)}, nil
		}
		return domain.Snapshot{Hash: sha(normalize(string(b))), Excerpt: truncate(string(b), 400), Location: file + ":1"}, nil
	}
	secs, err := m.sections(file)
	if err != nil {
		return domain.Snapshot{Skipped: fmt.Sprintf("ファイルが読めない: %v", err)}, nil
	}
	var names []string
	for _, s := range secs {
		if s.heading == anchor {
			return domain.Snapshot{
				Hash:     sha(normalize(s.heading + "\n" + s.body)),
				Excerpt:  strings.TrimSpace(s.body),
				Location: fmt.Sprintf("%s:%d", file, s.line),
			}, nil
		}
		names = append(names, s.heading)
	}
	return domain.Snapshot{Skipped: fmt.Sprintf("見出し「%s」が %s にない。存在する見出し: %s", anchor, file, strings.Join(names, ", "))}, nil
}

// Units は見出しごとの ref を返す（coverage 用）。
func (m *Markdown) Units(_ context.Context, ref string) ([]string, error) {
	file, _ := SplitRef(ref)
	secs, err := m.sections(file)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, s := range secs {
		if s.level == 1 { // 文書タイトルは単位にしない
			continue
		}
		out = append(out, file+"#"+s.heading)
	}
	return out, nil
}
