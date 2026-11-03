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

// HoldoutRepo は holdout/<claim-id>.yaml を読み書きする。
// 期待値は out_sha256（正規化 JSON の SHA-256）でしか持たず、平文の out はエラーにする。
// 書き込み（Append）は spec-gate holdout add 用で、人間だけが使う。
type HoldoutRepo struct{ root string }

func NewHoldout(root string) *HoldoutRepo { return &HoldoutRepo{root: root} }

// holdoutFile は holdout ファイルの書式。examples だけを許す。
type holdoutFile struct {
	Examples []holdoutEntry `yaml:"examples"`
}

// holdoutEntry は holdout 1 件の書式。in・out はキーがあるかどうかを見るために yaml.Node で受ける。
type holdoutEntry struct {
	In        yaml.Node `yaml:"in"`
	Out       yaml.Node `yaml:"out"`
	OutSHA256 string    `yaml:"out_sha256"`
}

func (r *HoldoutRepo) Holdout(claimID string) ([]domain.HoldoutExample, error) {
	if err := checkHoldoutID(claimID); err != nil {
		return nil, err
	}
	name, b, err := r.read(claimID)
	if err != nil || b == nil {
		return nil, err
	}
	return decodeHoldout(b, name)
}

func (r *HoldoutRepo) All() (map[string][]domain.HoldoutExample, error) {
	entries, err := os.ReadDir(filepath.Join(r.root, holdoutDir))
	if errors.Is(err, os.ErrNotExist) {
		return map[string][]domain.HoldoutExample{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s/ を読めない", holdoutDir)
	}
	all := map[string][]domain.HoldoutExample{}
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

// Append は holdout/<claim-id>.yaml の examples の末尾に 1 件足し、ルート相対のパスを返す。
// 既に <claim-id>.yml があればそちらに足す（.yaml を別に作ると .yml が読まれなくなる）。
// ファイルが無ければ作る。既存のファイルが読めない（平文の out が残っているなど）なら書かない。
// コメントや既存の項目はそのまま残す。
func (r *HoldoutRepo) Append(claimID string, ex domain.HoldoutExample) (string, error) {
	if err := checkHoldoutID(claimID); err != nil {
		return "", err
	}
	if !domain.ValidOutputHash(ex.OutSHA256) {
		return "", errors.New("holdout: out_sha256 が SHA-256 の 16 進小文字 64 文字ではない")
	}
	name, b, err := r.read(claimID)
	if err != nil {
		return "", err
	}
	if b == nil {
		name = claimID + ".yaml"
	} else if _, err := decodeHoldout(b, name); err != nil {
		return "", err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return "", fmt.Errorf("%s/%s を読めない（内容は表示しない）", holdoutDir, name)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		// 空・コメントだけのファイル
		doc = yaml.Node{Kind: yaml.DocumentNode, HeadComment: doc.HeadComment, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	seq := examplesNode(doc.Content[0])
	entry, err := holdoutEntryNode(ex)
	if err != nil {
		return "", err
	}
	seq.Content = append(seq.Content, entry)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return "", fmt.Errorf("%s/%s を書けない: %w", holdoutDir, name, err)
	}
	if err := enc.Close(); err != nil {
		return "", fmt.Errorf("%s/%s を書けない: %w", holdoutDir, name, err)
	}
	dir := filepath.Join(r.root, holdoutDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	// 書きかけのファイルを残さないよう、一時ファイルに書いてから置き換える
	tmp, err := os.CreateTemp(dir, "."+claimID+".*.tmp")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, name)); err != nil {
		return "", err
	}
	return filepath.Join(holdoutDir, name), nil
}

// read は claim の holdout ファイルを .yaml → .yml の順に探して読む。無ければ "", nil, nil。
func (r *HoldoutRepo) read(claimID string) (string, []byte, error) {
	for _, ext := range []string{".yaml", ".yml"} {
		name := claimID + ext
		b, err := os.ReadFile(filepath.Join(r.root, holdoutDir, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", nil, fmt.Errorf("%s/%s を読めない", holdoutDir, name)
		}
		return name, b, nil
	}
	return "", nil, nil
}

func checkHoldoutID(claimID string) error {
	if claimID == "" || strings.ContainsAny(claimID, `/\`) || strings.Contains(claimID, "..") {
		return fmt.Errorf("holdout: claim id %q はファイル名に使えない", claimID)
	}
	return nil
}

// examplesNode はトップレベルの mapping から examples の sequence を探す。無ければ足す。
func examplesNode(root *yaml.Node) *yaml.Node {
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "examples" {
			v := root.Content[i+1]
			if v.Kind != yaml.SequenceNode {
				// examples: だけ（null）の場合
				*v = yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			}
			return v
		}
	}
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "examples"}, seq)
	return seq
}

// holdoutEntryNode は {in: ..., out_sha256: "..."} の node を作る。
// out_sha256 は数字だけのハッシュが数値として読まれないよう引用符で囲む。
func holdoutEntryNode(ex domain.HoldoutExample) (*yaml.Node, error) {
	var in yaml.Node
	if err := in.Encode(ex.In); err != nil {
		return nil, fmt.Errorf("holdout: in を YAML にできない: %w", err)
	}
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "in"},
		&in,
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "out_sha256"},
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: ex.OutSHA256, Style: yaml.DoubleQuotedStyle},
	}}, nil
}

// decodeHoldout は holdout 1 ファイルを読む。未知のキーはエラー（綴り違いで holdout が黙って空になるのを防ぐ）。
// 平文の out はエラー（期待値はハッシュでしか持たない）。in が無い・out_sha256 の形が違うのもエラー。
// yaml のエラー文には値が含まれるので、エラーには行番号すら出さず、ファイル名と項目の番号だけを出す。
func decodeHoldout(b []byte, name string) ([]domain.HoldoutExample, error) {
	var f holdoutFile
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s/%s を読めない（examples: に in と out_sha256 だけを書く。内容は表示しない）", holdoutDir, name)
	}
	exs := make([]domain.HoldoutExample, 0, len(f.Examples))
	for i, e := range f.Examples {
		switch {
		case e.Out.Kind != 0:
			return nil, fmt.Errorf("%s/%s の examples[%d]: out は書けない（期待値は out_sha256 にハッシュで書く。spec-gate holdout add を使う。内容は表示しない）", holdoutDir, name, i)
		case e.In.Kind == 0:
			return nil, fmt.Errorf("%s/%s の examples[%d]: in が無い", holdoutDir, name, i)
		case !domain.ValidOutputHash(e.OutSHA256):
			return nil, fmt.Errorf("%s/%s の examples[%d]: out_sha256 が無いか、SHA-256 の 16 進小文字 64 文字ではない（内容は表示しない）", holdoutDir, name, i)
		}
		var in any
		if err := e.In.Decode(&in); err != nil {
			return nil, fmt.Errorf("%s/%s の examples[%d]: in を読めない（内容は表示しない）", holdoutDir, name, i)
		}
		exs = append(exs, domain.HoldoutExample{In: in, OutSHA256: e.OutSHA256})
	}
	return exs, nil
}
