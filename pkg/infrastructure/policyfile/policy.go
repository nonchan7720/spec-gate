// Package policyfile は自動承認の方針（.spec-gate/policy.yaml）を読む。書くのは人間だけ（エージェントは spec-gate guard が拒否する）。
package policyfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"gopkg.in/yaml.v3"
)

// RelPath は方針ファイル（プロジェクトルートから）。
const RelPath = ".spec-gate/policy.yaml"

// File は方針ファイル。
type File struct{ path string }

var _ domain.AutoPolicySource = File{}

// New は root の .spec-gate/policy.yaml を読む File を返す。
func New(root string) File { return File{path: filepath.Join(root, RelPath)} }

// Load はファイルの中身とそれを読んだ値を返す。無ければ (nil, nil, nil)。
func (f File) Load() (*domain.AutoPolicyFile, []byte, error) {
	raw, err := os.ReadFile(f.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var p domain.AutoPolicyFile
	if err := yaml.Unmarshal(raw, &p); err != nil {
		return nil, raw, fmt.Errorf("%s が YAML として読めない: %w", RelPath, err)
	}
	return &p, raw, nil
}
