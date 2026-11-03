// Package tracelog は claim の判定履歴（通り方のログ）を .spec-gate/trace.jsonl に読み書きする。
// git の管理対象外（.gitignore）のローカルな記録。
package tracelog

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// RelPath はプロジェクトルートからの記録ファイルのパス。
const RelPath = ".spec-gate/trace.jsonl"

// JSONL は判定 1 件を JSON 1 行で追記するログ。
type JSONL struct{ path string }

var _ domain.TraceLog = (*JSONL)(nil)

func New(root string) *JSONL { return &JSONL{path: filepath.Join(root, RelPath)} }

// Append は判定を 1 件 1 行で追記する。1 回の書き込みにまとめ、途中の行だけが残りにくくする。
func (l *JSONL) Append(entries []domain.TraceEntry) error {
	if len(entries) == 0 {
		return nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	_, werr := f.Write(buf.Bytes())
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	return werr
}

// All は記録を古い順に返す。読めない行（書き込み中の中断など）は飛ばす。ファイルが無ければ空。
func (l *JSONL) All() ([]domain.TraceEntry, error) {
	b, err := os.ReadFile(l.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []domain.TraceEntry
	for line := range bytes.Lines(b) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var e domain.TraceEntry
		if err := json.Unmarshal(line, &e); err != nil || e.ID == "" {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}
