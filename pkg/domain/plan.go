package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// PlanItem はプランファイルのチェック項目 1 つ。
type PlanItem struct {
	Text   string
	Verify string // 空なら verify 無し → fail
	Line   int
	Done   bool // "- [x]"
}

// Plan はプランファイル。
type Plan struct {
	Path   string
	Title  string
	Status string   // draft | active | done
	Claims []string // 満たすべき claim の ID
	Items  []PlanItem
}

// Active は check の対象になるプランか。
func (p Plan) Active() bool { return p.Status == "active" }

// MissingVerify は verify の無い項目。
func (p Plan) MissingVerify() []PlanItem {
	var out []PlanItem
	for _, it := range p.Items {
		if strings.TrimSpace(it.Verify) == "" {
			out = append(out, it)
		}
	}
	return out
}

// VerifyHash は verify 行だけから作るハッシュ。凍結後に書き換えられたら変わる。
// チェック項目の文言やチェック状態（[ ]/[x]）は含めない。
func (p Plan) VerifyHash() string {
	h := sha256.New()
	for _, it := range p.Items {
		_, _ = fmt.Fprintf(h, "%s\n", strings.TrimSpace(it.Verify))
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// PlanLock はプランごとの凍結記録。
type PlanLock struct {
	Hash     string `json:"hash"`
	FrozenAt string `json:"frozen_at"`
	FrozenBy string `json:"frozen_by"`
}
