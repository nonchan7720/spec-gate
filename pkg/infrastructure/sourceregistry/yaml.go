// Package sourceregistry は sources.yaml を読む。
package sourceregistry

import (
	"os"
	"path/filepath"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"gopkg.in/yaml.v3"
)

type Registry struct{ root string }

func New(root string) *Registry { return &Registry{root: root} }

func (r *Registry) All() ([]domain.SourceEntry, error) {
	b, err := os.ReadFile(filepath.Join(r.root, "sources.yaml"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc struct {
		Sources []domain.SourceEntry `yaml:"sources"`
	}
	return doc.Sources, yaml.Unmarshal(b, &doc)
}
