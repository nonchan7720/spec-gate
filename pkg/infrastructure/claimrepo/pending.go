package claimrepo

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// confidenceLine は SavePending が書くヘッダ `# confidence: 2/3`。
var confidenceLine = regexp.MustCompile(`(?m)^#\s*confidence:\s*(\S+)\s*$`)

// dryRunLine は試し実行の結果のヘッダ `# dry-run: pass`（docs/spec/harness.md「抽出結果の試し実行」）。
var dryRunLine = regexp.MustCompile(`(?m)^#\s*dry-run:[ \t]*(.*?)[ \t]*$`)

func (r *Repo) pendingDir() string { return filepath.Join(r.dir(), pendingDir) }

// Pending は claims/_pending/*.yaml を名前順に返す。
// 読めない候補もエラーつきで返す（一覧から黙って消すと、人間が却下する機会もなくなる）。
func (r *Repo) Pending() ([]domain.PendingClaim, error) {
	return readClaimDir(r, r.pendingDir(), func(f claimFile) domain.PendingClaim {
		return domain.PendingClaim{
			Name: f.name, Path: f.path, Claim: f.claim, Error: f.err,
			Confidence: f.header(confidenceLine), DryRun: f.header(dryRunLine),
		}
	})
}

// claimFile は check の対象外の置き場所（_pending / _ambiguous）にある claim ファイル 1 つ。
type claimFile struct {
	name  string       // ファイル名
	path  string       // ルートからの相対パス
	body  []byte       // ファイルの中身（ヘッダのコメントを読むため）
	claim domain.Claim // 読めた claim（File は絶対パス）
	err   string       // 読めない・検証に通らないときの理由
}

// header はファイルのヘッダのコメント（re の 1 つ目のグループ）。無ければ空。
func (f claimFile) header(re *regexp.Regexp) string {
	if m := re.FindSubmatch(f.body); m != nil {
		return string(m[1])
	}
	return ""
}

// readClaimDir は dir 直下の *.yaml を名前順（os.ReadDir の順）に読み、conv で変換して返す。dir が無ければ nil。
// 読めない claim もエラーつきで返す。ファイル自体が読めないときだけ error。
func readClaimDir[T any](r *Repo, dir string, conv func(claimFile) T) ([]T, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []T
	for _, e := range entries {
		if e.IsDir() || !isYAML(e.Name()) {
			continue
		}
		p := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		f := claimFile{name: e.Name(), path: rel(r.root, p), body: b}
		c, err := decodeClaim(b)
		if err == nil {
			err = c.Validate()
		}
		if err != nil {
			f.err = err.Error()
		}
		c.File = p
		f.claim = c
		out = append(out, conv(f))
	}
	return out, nil
}

// Promote は候補を claims/<area>/ に移す。移動先に同名ファイルがあれば上書きせずエラーにする。
func (r *Repo) Promote(name, area string) (string, error) {
	if !safeSegment(name) || !isYAML(name) {
		return "", fmt.Errorf("候補名が不正: %q", name)
	}
	if !safeSegment(area) || area == pendingDir || area == ambiguousDir {
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
	if err != nil || relp == "." || strings.HasPrefix(relp, "..") || filepath.Dir(relp) == pendingDir || filepath.Dir(relp) == ambiguousDir {
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

// WriteDryRun は候補 name の `# dry-run:` ヘッダを summary にする。無ければ `# confidence:` の次（それも無ければ先頭）に足す。
func (r *Repo) WriteDryRun(name, summary string) error {
	if !safeSegment(name) || !isYAML(name) {
		return fmt.Errorf("候補名が不正: %q", name)
	}
	summary = strings.ReplaceAll(strings.TrimSpace(summary), "\n", " ")
	p := filepath.Join(r.pendingDir(), name)
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	line := "# dry-run: " + summary
	var out string
	switch {
	case dryRunLine.Match(b):
		loc := dryRunLine.FindIndex(b)
		out = string(b[:loc[0]]) + line + string(b[loc[1]:])
	case confidenceLine.Match(b):
		loc := confidenceLine.FindIndex(b)
		out = string(b[:loc[1]]) + "\n" + line + string(b[loc[1]:])
	default:
		out = line + "\n" + string(b)
	}
	return os.WriteFile(p, []byte(out), 0o644)
}
