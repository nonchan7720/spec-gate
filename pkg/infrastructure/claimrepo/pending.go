package claimrepo

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// confidenceLine は SavePending が書くヘッダ `# confidence: 2/3`。
var confidenceLine = regexp.MustCompile(`(?m)^#\s*confidence:\s*(\S+)\s*$`)

func (r *Repo) pendingDir() string { return filepath.Join(r.dir(), pendingDir) }

// Pending は claims/_pending/*.yaml を名前順に返す。
// 読めない候補もエラーつきで返す（一覧から黙って消すと、人間が却下する機会もなくなる）。
func (r *Repo) Pending() ([]domain.PendingClaim, error) {
	entries, err := os.ReadDir(r.pendingDir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []domain.PendingClaim
	for _, e := range entries {
		if e.IsDir() || !isYAML(e.Name()) {
			continue
		}
		p := filepath.Join(r.pendingDir(), e.Name())
		pc := domain.PendingClaim{Name: e.Name(), Path: rel(r.root, p)}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		if m := confidenceLine.FindSubmatch(b); m != nil {
			pc.Confidence = string(m[1])
		}
		c, err := decodeClaim(b)
		if err == nil {
			err = c.Validate()
		}
		if err != nil {
			pc.Error = err.Error()
		}
		c.File = p
		pc.Claim = c
		out = append(out, pc)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Promote は候補を claims/<area>/ に移す。移動先に同名ファイルがあれば上書きせずエラーにする。
func (r *Repo) Promote(name, area string) (string, error) {
	if !safeSegment(name) || !isYAML(name) {
		return "", fmt.Errorf("候補名が不正: %q", name)
	}
	if !safeSegment(area) || area == pendingDir {
		return "", fmt.Errorf("領域名が不正: %q", area)
	}
	src := filepath.Join(r.pendingDir(), name)
	if _, err := os.Stat(src); err != nil {
		return "", fmt.Errorf("候補 %s がない: %w", name, err)
	}
	dstDir := filepath.Join(r.dir(), area)
	dst := filepath.Join(dstDir, name)
	if _, err := os.Lstat(dst); err == nil {
		return "", fmt.Errorf("既に %s がある", rel(r.root, dst))
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(src, dst); err != nil {
		return "", err
	}
	return dst, nil
}

// Demote は Promote で移したファイルを _pending に戻す。claims/ の外や _pending 内のファイルは扱わない。
func (r *Repo) Demote(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	relp, err := filepath.Rel(r.dir(), abs)
	if err != nil || relp == "." || strings.HasPrefix(relp, "..") || filepath.Dir(relp) == pendingDir {
		return fmt.Errorf("claims/<領域>/ のファイルではない: %s", path)
	}
	dst := filepath.Join(r.pendingDir(), filepath.Base(abs))
	if _, err := os.Lstat(dst); err == nil {
		return fmt.Errorf("既に %s がある", rel(r.root, dst))
	}
	if err := os.MkdirAll(r.pendingDir(), 0o755); err != nil {
		return err
	}
	return os.Rename(abs, dst)
}

// DiscardPending は候補を削除する（却下）。
func (r *Repo) DiscardPending(name string) error {
	if !safeSegment(name) || !isYAML(name) {
		return fmt.Errorf("候補名が不正: %q", name)
	}
	return os.Remove(filepath.Join(r.pendingDir(), name))
}

// safeSegment はパス区切りや . / .. を含まない 1 階層の名前か。
func safeSegment(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, `/\`) && filepath.Base(s) == s
}

func isYAML(name string) bool {
	return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
}
