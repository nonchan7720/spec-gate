package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/services"
)

func TestWriteReverseReport(t *testing.T) {
	rep := services.ReverseReport{Results: []domain.ReverseResult{
		{ID: "A-1", Title: "一致", Status: domain.ReverseMatch, Restated: "円未満は切り捨てる。", Spec: "spec.md:3"},
		{ID: "A-2", Title: "不一致", Status: domain.ReverseMismatch, Restated: "1 行目\n2 行目", Differences: []string{"負数の扱いが無い"}},
		{ID: "UI-1", Title: "画面", Status: domain.ReverseSkipped, Detail: "source type figma はテキストを持たない"},
		{ID: "A-3", Title: "壊れた", Status: domain.ReverseError, Detail: "応答に JSON が無い"},
	}}
	var buf bytes.Buffer
	writeReverseReport(&buf, rep)
	out := buf.String()
	for _, want := range []string{
		"✓ A-1", "[spec.md:3]", "円未満は切り捨てる。",
		"✗ A-2", "- 負数の扱いが無い", "      2 行目", // 複数行の再生成文もインデントを揃える
		"– UI-1", "figma",
		"! A-3", "JSON が無い",
		"1 match, 1 mismatch, 1 skipped, 1 error",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("出力に %q が無い:\n%s", want, out)
		}
	}
}
