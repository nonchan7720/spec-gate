// Package sourcefile は mutation score 用に、claim が触るソースファイルを探して読み書きする。
package sourcefile

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Files はプロジェクトルート配下のソースファイル。
type Files struct{ root string }

func New(root string) *Files { return &Files{root: root} }

// sourceExts は変異を入れる対象の拡張子（演算子の書き方が Go / JS 系のもの）。
var sourceExts = map[string]bool{
	".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true, ".cjs": true,
}

// skipDirs はディレクトリを展開するときに降りない名前。
var skipDirs = map[string]bool{"node_modules": true, "vendor": true, "testdata": true}

// isSource はテストファイルでないソースファイルか。
func isSource(rel string) bool {
	base := filepath.Base(rel)
	if !sourceExts[filepath.Ext(base)] {
		return false
	}
	if strings.HasSuffix(base, "_test.go") {
		return false
	}
	for _, s := range []string{".test.", ".spec."} {
		if strings.Contains(base, s) {
			return false
		}
	}
	return true
}

// Expand はパス（ファイル・ディレクトリ）をソースファイルに展開する。
// ルートの外・実在しないパス・テストファイルは除き、重複を除いて名前順で返す。
func (f *Files) Expand(paths []string) ([]string, error) {
	seen := map[string]bool{}
	for _, p := range paths {
		rel := filepath.Clean(filepath.FromSlash(p))
		if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		abs := filepath.Join(f.root, rel)
		st, err := os.Stat(abs)
		if err != nil {
			continue
		}
		if !st.IsDir() {
			if isSource(rel) {
				seen[filepath.ToSlash(rel)] = true
			}
			continue
		}
		err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if path != abs && (strings.HasPrefix(d.Name(), ".") || skipDirs[d.Name()]) {
					return filepath.SkipDir
				}
				return nil
			}
			r, err := filepath.Rel(f.root, path)
			if err != nil {
				return err
			}
			if d.Type().IsRegular() && isSource(r) {
				seen[filepath.ToSlash(r)] = true
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}

func (f *Files) Read(path string) (string, error) {
	b, err := os.ReadFile(filepath.Join(f.root, filepath.FromSlash(path)))
	return string(b), err
}

// Write は同じディレクトリの一時ファイルに書いてから置き換える（途中で止まっても半端な中身を残さない）。
// パーミッションは元のファイルに合わせる。
func (f *Files) Write(path, content string) error {
	abs := filepath.Join(f.root, filepath.FromSlash(path))
	st, err := os.Stat(abs)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(abs), ".spec-gate-mutate-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }() // rename 後は存在しないので無害
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, st.Mode().Perm()); err != nil {
		return err
	}
	return os.Rename(name, abs)
}
