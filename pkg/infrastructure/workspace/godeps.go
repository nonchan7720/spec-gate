package workspace

import (
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

// GoDeps は domain.DependencyResolver の実装。Go パッケージが直接・間接に import する
// モジュール内のパッケージのディレクトリを `go list -deps` で調べる。
// 結果はディレクトリごとにキャッシュする（1 回の実行の中で同じパッケージを何度も調べない）。
type GoDeps struct {
	root string
	// list は dir（root からの相対ディレクトリ）の依存パッケージのディレクトリ（絶対パス）を返す。テストで差し替える。
	list func(dir string) ([]string, error)

	mu    sync.Mutex
	cache map[string][]string
}

// NewGoDeps は root（モジュールのルート）を基準に依存を調べる GoDeps を作る。
func NewGoDeps(root string) *GoDeps {
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	d := &GoDeps{root: root, cache: map[string][]string{}}
	d.list = d.goList
	return d
}

// Deps は p が Go パッケージのディレクトリ（または Go ファイル）なら、そのパッケージが直接・間接に import する
// モジュール内のパッケージのディレクトリ（root からの相対パス。自分自身を含む）を返す。
// Go でないパス、リポジトリの外のパス、調べられないとき（go がない・モジュールが壊れている）は nil。
func (d *GoDeps) Deps(p string) []string {
	dir, ok := d.packageDir(p)
	if !ok {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if got, ok := d.cache[dir]; ok {
		return append([]string(nil), got...)
	}
	var out []string
	if dirs, err := d.list(dir); err == nil {
		out = d.relativeInside(dirs)
	}
	d.cache[dir] = out
	return append([]string(nil), out...)
}

// packageDir は p を Go パッケージのディレクトリ（root からの相対、/ 区切り）にする。
// Go ファイルならそのディレクトリ、ディレクトリなら直下に Go ファイルがあるときだけ。
func (d *GoDeps) packageDir(p string) (string, bool) {
	rel := filepath.FromSlash(strings.TrimSpace(p))
	if rel == "" || !filepath.IsLocal(rel) {
		return "", false
	}
	rel = filepath.Clean(rel)
	info, err := os.Stat(filepath.Join(d.root, rel))
	if err != nil {
		return "", false
	}
	if !info.IsDir() {
		if filepath.Ext(rel) != ".go" {
			return "", false
		}
		rel = filepath.Dir(rel)
	} else if m, _ := filepath.Glob(filepath.Join(d.root, rel, "*.go")); len(m) == 0 {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// relativeInside は絶対パスのディレクトリのうち root の中にあるものを、root からの相対パス（/ 区切り）にする。
func (d *GoDeps) relativeInside(dirs []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, abs := range dirs {
		if r, err := filepath.EvalSymlinks(abs); err == nil {
			abs = r
		}
		rel, err := filepath.Rel(d.root, abs)
		if err != nil || !filepath.IsLocal(rel) && rel != "." {
			continue
		}
		rel = path.Clean(filepath.ToSlash(rel))
		if !seen[rel] {
			seen[rel] = true
			out = append(out, rel)
		}
	}
	return out
}

// goList は `go list -deps -f '{{.Dir}}' ./<dir>` を root で実行する。
func (d *GoDeps) goList(dir string) ([]string, error) {
	cmd := exec.Command("go", "list", "-deps", "-f", "{{.Dir}}", "./"+dir)
	cmd.Dir = d.root
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var dirs []string
	for l := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			dirs = append(dirs, l)
		}
	}
	return dirs, nil
}
