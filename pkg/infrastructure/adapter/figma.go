package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// Figma は `figma://<fileKey>/<nodeId>` で指したノード部分木を仕様とみなす。
// 環境変数 FIGMA_TOKEN が無ければ Skipped を返す（claim 側の drift: optional と組み合わせる）。
type Figma struct {
	client  *http.Client
	baseURL string
}

func NewFigma() *Figma {
	return &Figma{client: http.DefaultClient, baseURL: "https://api.figma.com/v1"}
}

func (Figma) Type() domain.SourceType { return domain.SourceFigma }

func parseFigmaRef(ref string) (fileKey, nodeID string, err error) {
	s := strings.TrimPrefix(ref, "figma://")
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("figma ref は figma://<fileKey>/<nodeId> 形式: %s", ref)
	}
	return parts[0], parts[1], nil
}

func (f *Figma) fetchNode(ctx context.Context, ref string) (map[string]any, error) {
	token := os.Getenv("FIGMA_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("FIGMA_TOKEN が未設定")
	}
	key, node, err := parseFigmaRef(ref)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/files/%s/nodes?ids=%s", f.baseURL, key, strings.ReplaceAll(node, ":", "%3A"))
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("X-Figma-Token", token)
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("figma API %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var doc struct {
		Nodes map[string]struct {
			Document map[string]any `json:"document"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	for _, n := range doc.Nodes {
		return n.Document, nil
	}
	return nil, fmt.Errorf("ノード %s が見つからない", node)
}

func (f *Figma) Snapshot(ctx context.Context, ref string) (domain.Snapshot, error) {
	node, err := f.fetchNode(ctx, ref)
	if err != nil {
		return domain.Snapshot{Skipped: err.Error()}, nil
	}
	// 位置情報は配置調整のたびに変わるので除外し、構造とテキストだけでハッシュを取る
	texts := collectText(node)
	sort.Strings(texts)
	stripped := strip(node, map[string]bool{"absoluteBoundingBox": true, "absoluteRenderBounds": true, "relativeTransform": true, "size": true})
	b, _ := json.Marshal(stripped)
	return domain.Snapshot{
		Hash:     sha(string(b)),
		Excerpt:  "テキスト: " + strings.Join(texts, " / "),
		Location: ref,
	}, nil
}

// Units はテキストノードと variant 名（coverage 用）。
func (f *Figma) Units(ctx context.Context, ref string) ([]string, error) {
	node, err := f.fetchNode(ctx, ref)
	if err != nil {
		return nil, err
	}
	var units []string
	for _, t := range collectText(node) {
		units = append(units, ref+"#text:"+t)
	}
	for _, v := range collectVariants(node) {
		units = append(units, ref+"#variant:"+v)
	}
	return units, nil
}

// children は Figma ノードの子のうち、ノードとして読めるものを返す。
func children(n map[string]any) []map[string]any {
	ch, _ := n["children"].([]any)
	out := make([]map[string]any, 0, len(ch))
	for _, c := range ch {
		if cm, ok := c.(map[string]any); ok {
			out = append(out, cm)
		}
	}
	return out
}

// walk はノードを深さ優先（自身 → 子）でたどり、各ノードで pick が返した文字列を集める。
func walk(n map[string]any, pick func(map[string]any) []string) []string {
	out := pick(n)
	for _, c := range children(n) {
		out = append(out, walk(c, pick)...)
	}
	return out
}

// collectText は TEXT ノードの文字列（画面の文言）。
func collectText(n map[string]any) []string {
	return walk(n, func(n map[string]any) []string {
		if s, ok := n["characters"].(string); ok && n["type"] == "TEXT" {
			return []string{s}
		}
		return nil
	})
}

// collectVariants は COMPONENT_SET 直下の子の名前（= 状態。story の export 名と対応）。
func collectVariants(n map[string]any) []string {
	return walk(n, func(n map[string]any) []string {
		if n["type"] != "COMPONENT_SET" {
			return nil
		}
		var names []string
		for _, c := range children(n) {
			if name, ok := c["name"].(string); ok {
				names = append(names, name)
			}
		}
		return names
	})
}

func strip(v any, drop map[string]bool) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, val := range x {
			if !drop[k] {
				out[k] = strip(val, drop)
			}
		}
		return out
	case []any:
		for i := range x {
			x[i] = strip(x[i], drop)
		}
	}
	return v
}
