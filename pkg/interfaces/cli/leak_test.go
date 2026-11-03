package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func leakProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "claims/spec/ROUND-001.yaml", `id: ROUND-001
kind: example
title: 円未満は切り捨て
source: { type: markdown, ref: "docs/s.md#端数" }
binding: node round.js
examples:
  - { in: 1234.56, out: 1234 }
  - { in: "円未満は切り捨て", out: 0 }
`)
	writeFile(t, root, "holdout/ROUND-001.yaml", "examples:\n  - { in: 31337.5, out: 31337 }\n")
	return root
}

func TestRunLeak_ReportsValuesAndLines(t *testing.T) {
	root := leakProject(t)
	writeFile(t, root, "src/round.ts", "export function round(x: number) {\n  if (x === 1234.56) return 1234\n  if (x === 31337.5) return 31337\n  return Math.floor(x)\n}\n")
	var out strings.Builder
	code, err := runLeak(&out, newApp(root), []string{filepath.Join(root, "src/round.ts")})
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 {
		t.Fatalf("見つかれば exit 1: %d\n%s", code, out.String())
	}
	s := out.String()
	for _, want := range []string{"src/round.ts:2  1234.56", "src/round.ts:2  1234", "src/round.ts:3  holdout の値（値は表示しない）"} {
		if !strings.Contains(s, want) {
			t.Fatalf("%q が無い:\n%s", want, s)
		}
	}
	if strings.Contains(s, "31337") {
		t.Fatalf("holdout の値は出さない:\n%s", s)
	}
}

func TestRunLeak_CleanAndExcluded(t *testing.T) {
	root := leakProject(t)
	writeFile(t, root, "src/round.ts", "export const round = (x: number) => Math.floor(x)\n")
	writeFile(t, root, "src/round.test.ts", "expect(round(1234.56)).toBe(1234)\n")
	writeFile(t, root, "docs/s.md", "1234.56 → 1234\n")
	var out strings.Builder
	// ルート相対のパスでも渡せる
	code, err := runLeak(&out, newApp(root), []string{
		filepath.Join(root, "src/round.ts"), filepath.Join(root, "src/round.test.ts"), filepath.Join(root, "docs/s.md"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("値が無い・対象外のファイルは exit 0: %d\n%s", code, out.String())
	}
}

func TestRunLeak_MissingFile(t *testing.T) {
	root := leakProject(t)
	var out strings.Builder
	if _, err := runLeak(&out, newApp(root), []string{filepath.Join(root, "nope.ts")}); err == nil {
		t.Fatal("読めないファイルはエラー")
	}
}
