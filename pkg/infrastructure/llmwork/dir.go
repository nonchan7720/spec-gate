// Package llmwork は LLM 処理をサブエージェントに委譲するときの作業ディレクトリ（.spec-gate-llm/）を読み書きする。
package llmwork

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// workDir は作業ディレクトリの名前（services.LLMWorkDir と同じ）。この外は触らない。
const workDir = ".spec-gate-llm"

// Dir はプロジェクトルート直下の .spec-gate-llm/ を扱う。
type Dir struct{ root string }

var _ domain.LLMWorkspace = Dir{}

func New(root string) Dir { return Dir{root: root} }

// check は rel が作業ディレクトリの中（作業ディレクトリそのものは含まない）を指すことを確かめる。
func check(rel string) error {
	clean := path.Clean(rel)
	if path.IsAbs(rel) || clean != rel || !strings.HasPrefix(clean, workDir+"/") {
		return fmt.Errorf("%q は %s/ の中のパスではない", rel, workDir)
	}
	return nil
}

func (d Dir) Abs(rel string) string { return filepath.Join(d.root, filepath.FromSlash(rel)) }

func (d Dir) Write(rel, content string) error {
	if err := check(rel); err != nil {
		return err
	}
	p := d.Abs(rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(content), 0o644)
}

func (d Dir) Read(rel string) (string, bool, error) {
	if err := check(rel); err != nil {
		return "", false, err
	}
	b, err := os.ReadFile(d.Abs(rel))
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return string(b), true, nil
}

// RemoveAll は rel 以下を消し、空になった親を作業ディレクトリまで片付ける。
func (d Dir) RemoveAll(rel string) error {
	if err := check(rel); err != nil {
		return err
	}
	if err := os.RemoveAll(d.Abs(rel)); err != nil {
		return err
	}
	for dir := path.Dir(rel); dir == workDir || strings.HasPrefix(dir, workDir+"/"); dir = path.Dir(dir) {
		// 空でなければ失敗するので、残っているものは消えない
		if err := os.Remove(d.Abs(dir)); err != nil {
			break
		}
	}
	return nil
}
