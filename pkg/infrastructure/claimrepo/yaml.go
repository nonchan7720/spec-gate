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
			if d.Name() == pendingDir || d.Name() == ambiguousDir { // 承認待ち・曖昧の記録は check の対象外
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
	return r.savePending(c, confidence, "")
}

// savePending は候補を _pending に置く。extra は confidence と案内の行の間に入れるヘッダ（改行で終わる）。
func (r *Repo) savePending(c domain.Claim, confidence, extra string) (string, error) {
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
	head := fmt.Sprintf("# confidence: %s\n%s# 承認するには内容を確認して claims/<領域>/ に移し、`spec-gate approve %s` を実行する\n", confidence, extra, c.ID)
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

var _ domain.AutoPlacer = (*Repo)(nil)

// PlaceApproved は承認済みの claim を claims/<area>/<ID>.yaml に置く。同名があれば上書きせずエラー。
func (r *Repo) PlaceApproved(c domain.Claim, area string) (string, error) {
	if !safeSegment(area) || area == pendingDir || area == ambiguousDir {
		return "", fmt.Errorf("領域名が不正: %q", area)
	}
	name := unsafeChars.ReplaceAllString(c.ID, "-") + ".yaml"
	if !safeSegment(name) || c.Approved == nil {
		return "", fmt.Errorf("承認済みの claim でない、または ID が不正: %q", c.ID)
	}
	dir := filepath.Join(r.dir(), area)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return "", err
	}
	p := filepath.Join(dir, name)
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", fmt.Errorf("%s に置けない: %w", rel(r.root, p), err)
	}
	_, werr := f.Write(b)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = os.Remove(p)
		return "", werr
	}
	return rel(r.root, p), nil
}

// SavePendingAuto は自動承認しなかった候補を _pending に置き、合意数・試し実行・判定をヘッダに書く。
func (r *Repo) SavePendingAuto(c domain.Claim, confidence, dryRun, verdict string) (string, error) {
	extra := ""
	if dryRun != "" {
		extra += "# dry-run: " + strings.ReplaceAll(dryRun, "\n", " ") + "\n"
	}
	extra += "# auto: " + verdict + "\n"
	return r.savePending(c, confidence, extra)
}
