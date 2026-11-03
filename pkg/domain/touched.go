package domain

import (
	"path"
	"strings"
)

// PathChecker はリポジトリ内のパスの実在確認（ポート）。変更検知で binding の語を触るパスにするか決める。
type PathChecker interface {
	// Exists は、リポジトリのルートからの相対パス path がリポジトリ内に実在するファイルまたはディレクトリなら true。
	Exists(path string) bool
}

// IsTouched は、変更ファイル changed が claim の触るパス touches に関係するかを判定する。
// 実在の確認は済んでいる前提の純粋関数。見逃しを避ける側に倒す。
//
//   - 触るファイルそのもの
//   - 触るディレクトリの配下
//   - 触るファイルと同じディレクトリにあり、拡張子が同じ（同じパッケージの別ファイル）
func IsTouched(touches []string, changed string) bool {
	ch, ok := cleanPath(changed)
	if !ok {
		return false
	}
	for _, t := range touches {
		p, ok := cleanPath(t)
		if !ok {
			continue
		}
		if ch == p || strings.HasPrefix(ch, p+"/") {
			return true
		}
		if ext := path.Ext(p); ext != "" && path.Ext(ch) == ext && path.Dir(ch) == path.Dir(p) {
			return true
		}
	}
	return false
}

// cleanPath はパスを比較用にそろえる（区切りは /、先頭の ./ と末尾の / を外す）。URL と空は対象外。
func cleanPath(p string) (string, bool) {
	p = strings.ReplaceAll(strings.TrimSpace(p), `\`, "/")
	if p == "" || strings.Contains(p, "://") {
		return "", false
	}
	return path.Clean(p), true
}
