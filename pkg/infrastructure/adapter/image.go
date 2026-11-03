package adapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// Image は PNG / PDF など「中身を機械的に読めない」ソース。
// ファイルハッシュでドリフトだけ検知する。claim 候補の抽出は Vision 経由で別途行う。
type Image struct{ root string }

func NewImage(root string) *Image { return &Image{root: root} }

func (Image) Type() domain.SourceType { return domain.SourceImage }

func (m *Image) Snapshot(_ context.Context, ref string) (domain.Snapshot, error) {
	f, err := os.Open(filepath.Join(m.root, ref))
	if err != nil {
		return domain.Snapshot{Skipped: fmt.Sprintf("ファイルが読めない: %v", err)}, nil
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return domain.Snapshot{Skipped: err.Error()}, nil
	}
	return domain.Snapshot{
		Hash:     "sha256:" + hex.EncodeToString(h.Sum(nil)),
		Excerpt:  "(バイナリ。変更検知のみ)",
		Location: ref,
	}, nil
}

// Units は画像全体を 1 単位とする。
func (m *Image) Units(_ context.Context, ref string) ([]string, error) {
	return []string{ref}, nil
}
