package adapter

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeSpec(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const specMD = "# 金額計算\n\n## 端数処理\n\n円未満は切り捨てる。\n\n### 例\n\n1234.56 → 1234\n\n## マイナス金額\n\n0 にする。\n"

func snap(t *testing.T, root, ref string) (hash, location, skipped string) {
	t.Helper()
	s, err := NewMarkdown(root).Snapshot(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	return s.Hash, s.Location, s.Skipped
}

func TestMarkdownSnapshot_SectionHashAndLocation(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, "spec.md", specMD)
	h, loc, skipped := snap(t, root, "spec.md#端数処理")
	if h == "" || skipped != "" || loc != "spec.md:3" {
		t.Fatalf("hash=%q loc=%q skipped=%q", h, loc, skipped)
	}
}

func TestMarkdownSnapshot_WhitespaceDoesNotStale(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, "spec.md", specMD)
	before, _, _ := snap(t, root, "spec.md#端数処理")
	writeSpec(t, root, "spec.md", strings.Replace(specMD, "円未満は切り捨てる。\n", "円未満は切り捨てる。   \n\n\n\n", 1))
	after, _, _ := snap(t, root, "spec.md#端数処理")
	if before != after {
		t.Fatal("行末の空白や空行の数だけの差では stale にしない")
	}
}

func TestMarkdownSnapshot_SubsectionChangeStalesParent(t *testing.T) {
	// 「## 端数処理」の下の「### 例」も仕様の一部。ここを書き換えても stale にならないと仕様変更を見逃す
	root := t.TempDir()
	writeSpec(t, root, "spec.md", specMD)
	before, _, _ := snap(t, root, "spec.md#端数処理")
	writeSpec(t, root, "spec.md", strings.Replace(specMD, "1234.56 → 1234", "1234.56 → 1235", 1))
	after, _, _ := snap(t, root, "spec.md#端数処理")
	if before == after {
		t.Fatal("小見出しの中の変更も親の節のハッシュを変える")
	}
	other, _, _ := snap(t, root, "spec.md#マイナス金額")
	writeSpec(t, root, "spec.md", specMD)
	otherBefore, _, _ := snap(t, root, "spec.md#マイナス金額")
	if other != otherBefore {
		t.Fatal("別の節（同じレベルの次の見出し以降）は影響を受けない")
	}
}

func TestMarkdownSnapshot_MissingHeadingListsHeadings(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, "spec.md", specMD)
	_, _, skipped := snap(t, root, "spec.md#締め処理")
	if !strings.Contains(skipped, "締め処理") || !strings.Contains(skipped, "端数処理") {
		t.Fatalf("無い見出しは、存在する見出しを添えて skipped: %q", skipped)
	}
}

func TestMarkdownSnapshot_MissingHeadingSuggestsClosest(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, "spec.md", specMD)
	s, err := NewMarkdown(root).Snapshot(context.Background(), "spec.md#端数の処理")
	if err != nil {
		t.Fatal(err)
	}
	if s.Skipped == "" || s.Suggestion != "端数処理" {
		t.Fatalf("見出しが無いときは最も近い見出しを提案する: %+v", s)
	}

	s, err = NewMarkdown(root).Snapshot(context.Background(), "spec.md#変更履歴")
	if err != nil {
		t.Fatal(err)
	}
	if s.Suggestion != "" {
		t.Fatalf("遠い見出ししか無ければ提案しない: %q", s.Suggestion)
	}
}

func TestMarkdownSnapshot_FoundHeadingHasNoSuggestion(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, "spec.md", specMD)
	s, err := NewMarkdown(root).Snapshot(context.Background(), "spec.md#端数処理")
	if err != nil {
		t.Fatal(err)
	}
	if s.Skipped != "" || s.Suggestion != "" {
		t.Fatalf("見出しがあれば提案は付けない: %+v", s)
	}
}

func TestMarkdownSnapshot_HeadingInCodeFenceIsNotASection(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, "spec.md", "## A\n\n```sh\n# comment\n```\n\nbody\n")
	_, _, skipped := snap(t, root, "spec.md#comment")
	if skipped == "" {
		t.Fatal("コードブロック内の # は見出しではない")
	}
}

func TestMarkdownUnits_SkipsDocumentTitle(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, "spec.md", specMD)
	units, err := NewMarkdown(root).Units(context.Background(), "spec.md")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"spec.md#端数処理", "spec.md#例", "spec.md#マイナス金額"}
	if !reflect.DeepEqual(units, want) {
		t.Fatalf("units = %v, want %v", units, want)
	}
}

func TestMarkdownSnapshot_UnreadableFileIsSkipped(t *testing.T) {
	_, _, skipped := snap(t, t.TempDir(), "missing.md#a")
	if skipped == "" {
		t.Fatal("読めないファイルは skipped（drift: required の claim は service が fail にする）")
	}
}
