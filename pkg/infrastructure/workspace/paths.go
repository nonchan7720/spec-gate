// Package workspace はリポジトリ（作業ツリー）のファイルシステムを見る。
package workspace

import (
	"os"
	"path/filepath"
	"strings"
)

// Paths は domain.PathChecker の実装。Root からの相対パスが実在するかを答える。
type Paths struct{ Root string }

// Exists は path がリポジトリ内に実在するファイルまたはディレクトリなら true。
// 絶対パスと、リポジトリの外を指すパス（../ で抜けるもの）は false。
func (p Paths) Exists(path string) bool {
	rel := filepath.FromSlash(strings.TrimSpace(path))
	if rel == "" || !filepath.IsLocal(rel) {
		return false
	}
	_, err := os.Stat(filepath.Join(p.Root, rel))
	return err == nil
}
