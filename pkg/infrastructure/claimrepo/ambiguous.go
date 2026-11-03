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

// ambiguousDir は「仕様が決まっていない」と記録した候補の置き場所。_pending と同じく check の対象外。
const ambiguousDir = "_ambiguous"

// ambiguousLine は MarkAmbiguous が書くヘッダ `# ambiguous: <理由>`。
var ambiguousLine = regexp.MustCompile(`(?m)^#\s*ambiguous:[ \t]*(.*?)[ \t]*$`)

func (r *Repo) ambiguousDir() string { return filepath.Join(r.dir(), ambiguousDir) }

// MarkAmbiguous は候補を claims/_ambiguous/ に移し、先頭に `# ambiguous: <理由>` を書き足す。
// 理由は 1 行にまとめる（改行が入るとヘッダが壊れ、YAML 本体に混ざる）。
func (r *Repo) MarkAmbiguous(name, reason string) (string, error) {
	reason = strings.Join(strings.Fields(reason), " ")
	if reason == "" {
		return "", fmt.Errorf("曖昧とする理由が空")
	}
	if !safeSegment(name) || !isYAML(name) {
		return "", fmt.Errorf("候補名が不正: %q", name)
	}
	src := filepath.Join(r.pendingDir(), name)
	b, err := os.ReadFile(src)
	if err != nil {
		return "", fmt.Errorf("候補 %s がない: %w", name, err)
	}
	dst := filepath.Join(r.ambiguousDir(), name)
	if _, err := os.Lstat(dst); err == nil {
		return "", fmt.Errorf("既に %s がある", rel(r.root, dst))
	}
	if err := os.MkdirAll(r.ambiguousDir(), 0o755); err != nil {
		return "", err
	}
	head := "# ambiguous: " + reason + "\n"
	// O_EXCL: 確認と書き込みの間に同名ファイルができても上書きしない
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	_, werr := f.Write(append([]byte(head), b...))
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = os.Remove(dst)
		return "", werr
	}
	if err := os.Remove(src); err != nil {
		// 二重に残すより、移動を取り消して _pending に残す
		_ = os.Remove(dst)
		return "", err
	}
	return dst, nil
}

// Ambiguous は claims/_ambiguous/*.yaml を名前順に返す。読めないものもエラーつきで返す。
func (r *Repo) Ambiguous() ([]domain.AmbiguousClaim, error) {
	entries, err := os.ReadDir(r.ambiguousDir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []domain.AmbiguousClaim
	for _, e := range entries {
		if e.IsDir() || !isYAML(e.Name()) {
			continue
		}
		p := filepath.Join(r.ambiguousDir(), e.Name())
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		ac := domain.AmbiguousClaim{Name: e.Name(), Path: rel(r.root, p)}
		if m := ambiguousLine.FindSubmatch(b); m != nil {
			ac.Reason = string(m[1])
		}
		c, err := decodeClaim(b)
		if err == nil {
			err = c.Validate()
		}
		if err != nil {
			ac.Error = err.Error()
		}
		c.File = p
		ac.Claim = c
		out = append(out, ac)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
