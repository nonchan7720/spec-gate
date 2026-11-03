// Package domain は spec-gate の中核概念を定義する。
// 外部依存（ファイル・コマンド・ネットワーク）は持たず、ポート(interface)経由で infrastructure に委ねる。
package domain

import (
	"fmt"
	"strings"
)

// SourceType は仕様ソースの種類。
type SourceType string

const (
	SourceMarkdown SourceType = "markdown"
	SourceMermaid  SourceType = "mermaid"
	SourceFigma    SourceType = "figma"
	SourceImage    SourceType = "image"
)

// ClaimKind は claim の検証方式。
type ClaimKind string

const (
	KindExample   ClaimKind = "example"   // 入出力例と性質
	KindStructure ClaimKind = "structure" // 依存構造
	KindUI        ClaimKind = "ui"        // 画面の状態・文言
	KindManual    ClaimKind = "manual"    // 機械判定不可。常に unverified
)

// Source は claim が根拠とする仕様の位置。
type Source struct {
	Type SourceType `yaml:"type"`
	Ref  string     `yaml:"ref"`
	// Drift: "required"（既定）は snapshot が取れなければ fail、"optional" は警告のみ。
	Drift string `yaml:"drift,omitempty"`
}

func (s Source) DriftRequired() bool { return s.Drift != "optional" }

// Example は 1 つの入出力例。
type Example struct {
	In   any    `yaml:"in"`
	Out  any    `yaml:"out"`
	Note string `yaml:"note,omitempty"`
}

// Approval は人間が承認した記録。source_hash が現在の仕様と違えば stale。
type Approval struct {
	By         string `yaml:"by"`
	At         string `yaml:"at"`
	SourceHash string `yaml:"source_hash"`
}

// Claim は「仕様のうち機械的に検証できる形にした 1 つの主張」。
type Claim struct {
	ID       string    `yaml:"id"`
	Kind     ClaimKind `yaml:"kind"`
	Title    string    `yaml:"title"`
	Source   Source    `yaml:"source"`
	Approved *Approval `yaml:"approved,omitempty"`

	// kind: example
	// Binding は stdin で {"in": ...} を受け取り stdout に {"out": ...} を返すコマンド。
	Binding    string    `yaml:"binding,omitempty"`
	Examples   []Example `yaml:"examples,omitempty"`
	Properties []string  `yaml:"properties,omitempty"` // expr 式。input / output が使える
	// Fuzz を書くと、examples に加えてランダムな入力で properties と relations を評価する。
	Fuzz *Fuzz `yaml:"fuzz,omitempty"`
	// Relations はメタモルフィック関係（expr 式）。x は生成した入力、f(...) は binding の呼び出し。
	Relations []string `yaml:"relations,omitempty"`

	// kind: structure
	Layers    map[string]string `yaml:"layers,omitempty"`    // 層名 → ディレクトリ
	Allowed   string            `yaml:"allowed,omitempty"`   // "from-diagram"
	Forbidden []string          `yaml:"forbidden,omitempty"` // "a -> b"

	// kind: ui
	Story   string   `yaml:"story,omitempty"`
	States  []string `yaml:"states,omitempty"`
	URL     string   `yaml:"url,omitempty"`
	Strings []string `yaml:"strings,omitempty"`

	// File は読み込み元。永続化しない。
	File string `yaml:"-"`
}

// Validate は構造上の必須項目を確認する。意味の整合性は Lint が担う。
func (c Claim) Validate() error {
	switch {
	case c.ID == "":
		return fmt.Errorf("id がない")
	case c.Title == "":
		return fmt.Errorf("%s: title がない", c.ID)
	case c.Source.Type == "" || c.Source.Ref == "":
		return fmt.Errorf("%s: source.type / source.ref が必要", c.ID)
	}
	if c.Kind != KindExample && (c.Fuzz != nil || len(c.Relations) > 0) {
		return fmt.Errorf("%s: fuzz / relations は kind: example だけ", c.ID)
	}
	switch c.Kind {
	case KindExample:
		if c.Binding == "" {
			return fmt.Errorf("%s: example には binding が必要", c.ID)
		}
		if len(c.Examples) == 0 && len(c.Properties) == 0 && len(c.Relations) == 0 {
			return fmt.Errorf("%s: examples か properties が必要", c.ID)
		}
		if len(c.Relations) > 0 && c.Fuzz == nil {
			return fmt.Errorf("%s: relations は fuzz の入力で評価するので fuzz が必要", c.ID)
		}
		if c.Fuzz != nil {
			if err := c.Fuzz.validate(); err != nil {
				return fmt.Errorf("%s: %w", c.ID, err)
			}
			if len(c.Properties) == 0 && len(c.Relations) == 0 {
				return fmt.Errorf("%s: fuzz には評価する properties か relations が必要", c.ID)
			}
		}
	case KindStructure:
		if len(c.Layers) == 0 {
			return fmt.Errorf("%s: structure には layers が必要", c.ID)
		}
	case KindUI:
		if c.Story == "" && len(c.Strings) == 0 {
			return fmt.Errorf("%s: ui には story か strings が必要", c.ID)
		}
	case KindManual:
	default:
		return fmt.Errorf("%s: kind が不正: %q", c.ID, c.Kind)
	}
	return nil
}

// Touches は claim が参照するコード側のパス（変更検知用）。
// story と layers はそのまま。binding はコマンド文字列なので、その中の語のうち
// exists が「リポジトリ内に実在するファイルまたはディレクトリ」と答えたものを拾う。
// exists が nil（実在を確かめられない）なら、見逃さない側に倒してフラグと URL 以外の語をすべて拾う。
func (c Claim) Touches(exists func(path string) bool) []string {
	var p []string
	if c.Story != "" {
		p = append(p, c.Story)
	}
	for _, d := range c.Layers {
		p = append(p, d)
	}
	for _, w := range splitWords(c.Binding) {
		w = strings.Trim(w, `"'`)
		if w == "" || strings.HasPrefix(w, "-") || strings.Contains(w, "://") {
			continue
		}
		if rel, ok := cleanPath(w); ok && (exists == nil || exists(rel)) {
			p = append(p, rel)
		}
	}
	return p
}
