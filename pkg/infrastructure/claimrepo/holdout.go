package claimrepo

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"gopkg.in/yaml.v3"
)

// holdoutDir は holdout（エージェントに見せない入出力例）の置き場。claims/ と同じくプロジェクトルート直下。
const holdoutDir = "holdout"

// HoldoutRepo は holdout/<claim-id>.yaml を読む。書き込みは人間が行うので持たない。
type HoldoutRepo struct{ root string }

func NewHoldout(root string) *HoldoutRepo { return &HoldoutRepo{root: root} }

// holdoutFile は holdout ファイルの書式。examples だけを許す。
type holdoutFile struct {
	Examples []domain.Example `yaml:"examples"`
}

func (r *HoldoutRepo) Holdout(claimID string) ([]domain.Example, error) {
	if claimID == "" || strings.ContainsAny(claimID, `/\`) || strings.Contains(claimID, "..") {
		return nil, fmt.Errorf("holdout: claim id %q はファイル名に使えない", claimID)
	}
	for _, ext := range []string{".yaml", ".yml"} {
		name := claimID + ext
		b, err := os.ReadFile(filepath.Join(r.root, holdoutDir, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("%s/%s を読めない", holdoutDir, name)
		}
		return decodeHoldout(b, name)
	}
	return nil, nil
}

func (r *HoldoutRepo) All() (map[string][]domain.Example, error) {
	entries, err := os.ReadDir(filepath.Join(r.root, holdoutDir))
	if errors.Is(err, os.ErrNotExist) {
		return map[string][]domain.Example{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s/ を読めない", holdoutDir)
	}
	all := map[string][]domain.Example{}
	for _, e := range entries {
		if e.IsDir() || !isYAML(e.Name()) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(r.root, holdoutDir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("%s/%s を読めない", holdoutDir, e.Name())
		}
		exs, err := decodeHoldout(b, e.Name())
		if err != nil {
			return nil, err
		}
		id := strings.TrimSuffix(strings.TrimSuffix(e.Name(), ".yaml"), ".yml")
		all[id] = exs
	}
	return all, nil
}

// decodeHoldout は holdout 1 ファイルを読む。未知のキーはエラー（綴り違いで holdout が黙って空になるのを防ぐ）。
// yaml のエラー文には値が含まれるので、エラーには行番号すら出さず、ファイル名だけを出す。
func decodeHoldout(b []byte, name string) ([]domain.Example, error) {
	var f holdoutFile
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s/%s を読めない（examples: だけを書く。内容は表示しない）", holdoutDir, name)
	}
	return f.Examples, nil
}
