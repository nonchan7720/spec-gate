// Package claimrepo は claims/**/*.yaml を読み書きする。
package claimrepo

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"gopkg.in/yaml.v3"
)

const pendingDir = "_pending"

type Repo struct{ root string }

func New(root string) *Repo { return &Repo{root: root} }

func (r *Repo) dir() string { return filepath.Join(r.root, "claims") }

func (r *Repo) All() ([]domain.Claim, error) {
	var claims []domain.Claim
	ids := map[string]string{}
	err := filepath.WalkDir(r.dir(), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == pendingDir {
				return fs.SkipDir
			}
			return nil
		}
		if !isYAML(p) {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		c, err := decodeClaim(b)
		if err != nil {
			return fmt.Errorf("%s: %w", rel(r.root, p), err)
		}
		if err := c.Validate(); err != nil {
			return fmt.Errorf("%s: %w", rel(r.root, p), err)
		}
		if prev, dup := ids[c.ID]; dup {
			return fmt.Errorf("claim id %s が重複: %s と %s", c.ID, prev, rel(r.root, p))
		}
		ids[c.ID] = rel(r.root, p)
		c.File = p
		claims = append(claims, c)
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	sort.Slice(claims, func(i, j int) bool { return claims[i].ID < claims[j].ID })
	return claims, err
}

// decodeClaim は claim 1 件を読む。
// 未知のキーはエラーにする。綴り違い（forbiden など）を黙って無視すると、その制約が無いまま pass になる
func decodeClaim(b []byte) (domain.Claim, error) {
	var c domain.Claim
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil && !errors.Is(err, io.EOF) {
		return c, err
	}
	return c, nil
}

func (r *Repo) Save(c domain.Claim) error {
	if c.File == "" {
		return fmt.Errorf("%s: 保存先が不明", c.ID)
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(c.File, b, 0o644)
}

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func (r *Repo) SavePending(c domain.Claim, confidence string) (string, error) {
	dir := filepath.Join(r.dir(), pendingDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := unsafeChars.ReplaceAllString(c.ID, "-")
	if name == "" {
		name = unsafeChars.ReplaceAllString(c.Title, "-")
	}
	if name == "" {
		name = "claim"
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return "", err
	}
	head := fmt.Sprintf("# confidence: %s\n# 承認するには内容を確認して claims/<領域>/ に移し、`spec-gate approve %s` を実行する\n", confidence, c.ID)
	f, p, err := createUnique(dir, name)
	if err != nil {
		return "", err
	}
	if _, err := f.Write(append([]byte(head), b...)); err != nil {
		_ = f.Close()
		return "", err
	}
	return rel(r.root, p), f.Close()
}

// createUnique は dir/<name>.yaml を新規作成する。既にあれば <name>-2.yaml, <name>-3.yaml … と
// 空いている名前を探す。O_EXCL で作るので、既存の候補（前回の extract の分も含む）を上書きしない。
func createUnique(dir, name string) (*os.File, string, error) {
	for i := 1; ; i++ {
		n := name
		if i > 1 {
			n = fmt.Sprintf("%s-%d", name, i)
		}
		p := filepath.Join(dir, n+".yaml")
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			return f, p, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, "", err
		}
	}
}

func rel(root, p string) string {
	if r, err := filepath.Rel(root, p); err == nil {
		return r
	}
	return p
}
