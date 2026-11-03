// Package adapter は仕様ソース（Markdown / Mermaid / Figma / 画像）の読み方を実装する。
package adapter

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

func sha(s string) string {
	h := sha256.Sum256([]byte(s))
	return "sha256:" + hex.EncodeToString(h[:])
}

var multiBlank = regexp.MustCompile(`\n{3,}`)

// normalize は空白・空行の差で stale にならないよう整える。
func normalize(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t\r")
	}
	return strings.TrimSpace(multiBlank.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
