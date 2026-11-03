// Package claimrepo は claims/**/*.yaml を読み書きする。
package claimrepo

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

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
		if !strings.HasSuffix(p, ".yaml") && !strings.HasSuffix(p, ".yml") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		var c domain.Claim
		if err := yaml.Unmarshal(b, &c); err != nil {
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
	name := c.ID
	if name == "" {
		name = unsafeChars.ReplaceAllString(c.Title, "-")
	}
	p := filepath.Join(dir, name+".yaml")
	b, err := yaml.Marshal(c)
	if err != nil {
		return "", err
	}
	head := fmt.Sprintf("# confidence: %s\n# 承認するには内容を確認して claims/<領域>/ に移し、`spec-gate approve %s` を実行する\n", confidence, c.ID)
	return rel(r.root, p), os.WriteFile(p, append([]byte(head), b...), 0o644)
}

func rel(root, p string) string {
	if r, err := filepath.Rel(root, p); err == nil {
		return r
	}
	return p
}
