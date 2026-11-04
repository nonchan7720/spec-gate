// Package evidence は自動承認の証拠ファイル（.spec-gate/evidence/<ID>.json）の置き場所。
package evidence

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// RelDir は証拠を置くディレクトリ（プロジェクトルートから）。コミットする（docs/spec/auto-approval.md「証拠の照合」）。
const RelDir = domain.EvidenceRelDir

// Store は証拠ファイルを読み書きする。
type Store struct{ dir string }

var _ domain.EvidenceStore = (*Store)(nil)

// New は root の .spec-gate/evidence を使う Store を返す。
func New(root string) *Store { return &Store{dir: filepath.Join(root, RelDir)} }

func (s *Store) path(id string) (string, error) {
	if id == "" || strings.ContainsAny(id, `/\`) || strings.HasPrefix(id, ".") {
		return "", fmt.Errorf("claim ID が不正: %q", id)
	}
	return filepath.Join(s.dir, id+".json"), nil
}

// Load は証拠ファイルの中身を返す。無ければ ok=false。
func (s *Store) Load(id string) ([]byte, bool, error) {
	p, err := s.path(id)
	if err != nil {
		return nil, false, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return b, true, nil
}

// Save は証拠を整形した JSON で書き、その sha256 を返す。
func (s *Store) Save(e domain.Evidence) (string, error) {
	p, err := s.path(e.ID)
	if err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return "", err
	}
	b = append(b, '\n')
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		return "", err
	}
	return domain.SHA256Ref(b), nil
}
