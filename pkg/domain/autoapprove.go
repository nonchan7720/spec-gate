package domain

import (
	"slices"
	"strings"
)

// 承認の自動化（docs/spec/auto-approval.md）の、決定的に確かめられる部分。

// 自動承認の方針の既定値（「承認方針」）。
const (
	AutoDefaultRuns       = 3
	AutoRunsMin           = 2
	AutoRunsMax           = 10
	AutoDefaultWindow     = 20
	AutoDefaultMaxRevoked = 2
)

// AutoPolicyFile は .spec-gate/policy.yaml を読んだ値。書かれていない項目は nil（既定値で埋める）。
type AutoPolicyFile struct {
	AutoApprove *struct {
		Enabled        *bool    `json:"enabled" yaml:"enabled"`
		LLMCommands    []string `json:"llm_commands" yaml:"llm_commands"`
		Runs           *int     `json:"runs" yaml:"runs"`
		MinAgree       *int     `json:"min_agree" yaml:"min_agree"`
		Kinds          []string `json:"kinds" yaml:"kinds"`
		RequireReverse *bool    `json:"require_reverse" yaml:"require_reverse"`
		Breaker        *struct {
			Window     *int    `json:"window" yaml:"window"`
			MaxRevoked *int    `json:"max_revoked" yaml:"max_revoked"`
			ResetAt    *string `json:"reset_at" yaml:"reset_at"`
		} `json:"breaker" yaml:"breaker"`
	} `json:"auto_approve" yaml:"auto_approve"`
}

// AutoBreakerPolicy はブレーカーの方針。
type AutoBreakerPolicy struct {
	Window     int    `json:"window"`
	MaxRevoked int    `json:"max_revoked"`
	ResetAt    string `json:"reset_at"`
}

// AutoPolicy は既定値を埋めて確かめた方針。Enabled が false なら、ほかの項目は使わない。
type AutoPolicy struct {
	Enabled        bool              `json:"enabled"`
	LLMCommands    []string          `json:"llm_commands"`
	Runs           int               `json:"runs"`
	MinAgree       int               `json:"min_agree"`
	Kinds          []string          `json:"kinds"`
	RequireReverse bool              `json:"require_reverse"`
	Breaker        AutoBreakerPolicy `json:"breaker"`
}

// AutoPolicyError は方針の誤り。Field は最初に当てはまった項目名（llm_commands → runs → min_agree → kinds）。
type AutoPolicyError struct{ Field string }

func (e *AutoPolicyError) Error() string {
	return "policy.yaml の auto_approve." + e.Field + " が不正"
}

func valueOr[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}

// ResolveAutoPolicy は方針に既定値を埋めて確かめる。f が nil（ファイルが無い）か enabled が false なら、何も確かめず無効の方針を返す。
func ResolveAutoPolicy(f *AutoPolicyFile) (AutoPolicy, error) {
	if f == nil || f.AutoApprove == nil || !valueOr(f.AutoApprove.Enabled, false) {
		return AutoPolicy{}, nil
	}
	a := f.AutoApprove
	p := AutoPolicy{
		Enabled:        true,
		LLMCommands:    a.LLMCommands,
		Runs:           valueOr(a.Runs, AutoDefaultRuns),
		MinAgree:       valueOr(a.MinAgree, 0),
		Kinds:          a.Kinds,
		RequireReverse: valueOr(a.RequireReverse, true),
		Breaker:        AutoBreakerPolicy{Window: AutoDefaultWindow, MaxRevoked: AutoDefaultMaxRevoked},
	}
	if len(p.Kinds) == 0 {
		p.Kinds = []string{string(KindExample)}
	}
	if b := a.Breaker; b != nil {
		p.Breaker = AutoBreakerPolicy{
			Window:     valueOr(b.Window, AutoDefaultWindow),
			MaxRevoked: valueOr(b.MaxRevoked, AutoDefaultMaxRevoked),
			ResetAt:    valueOr(b.ResetAt, ""),
		}
	}
	switch {
	case len(p.LLMCommands) == 0:
		return AutoPolicy{}, &AutoPolicyError{"llm_commands"}
	case p.Runs < AutoRunsMin || p.Runs > AutoRunsMax:
		return AutoPolicy{}, &AutoPolicyError{"runs"}
	case p.MinAgree < 0 || p.MinAgree > p.Runs:
		return AutoPolicy{}, &AutoPolicyError{"min_agree"}
	}
	for _, k := range p.Kinds {
		if !slices.Contains([]ClaimKind{KindExample, KindStructure, KindUI, KindManual}, ClaimKind(k)) {
			return AutoPolicy{}, &AutoPolicyError{"kinds"}
		}
	}
	return p, nil
}

// AutoRunCommands は run ごとに使うコマンド。run k（1 始まり）は commands の先頭から数えて ((k−1) mod 個数) + 1 番目。
func AutoRunCommands(commands []string, runs int) []string {
	out := make([]string, 0, max(runs, 0))
	for k := 0; k < runs && len(commands) > 0; k++ {
		out = append(out, commands[k%len(commands)])
	}
	return out
}

// 自動承認の判定（「自動承認の判定」）。lint は呼び出し側で確かめる。
const (
	AutoOK        = "ok"
	AutoDisabled  = "disabled"
	AutoPaused    = "paused"
	AutoKind      = "kind"
	AutoAgree     = "agree"
	AutoAmbiguous = "ambiguous"
	AutoDryRun    = "dry-run"
	AutoReverse   = "reverse"
	AutoLint      = "lint"
	AutoExists    = "exists"
)

// AutoCandidate は判定に使う候補の情報。Reverse は逆変換の比較の結果（match / mismatch / error / skipped）。
type AutoCandidate struct {
	Kind       ClaimKind `json:"kind"`
	Title      string    `json:"title"`
	Confidence string    `json:"confidence"`
	DryRun     string    `json:"dry_run"`
	Reverse    string    `json:"reverse"`
}

// AutoEligibility は候補 1 件の自動承認の判定。最初に当てはまったものを返し、どれにも当てはまらなければ ok。
func AutoEligibility(p AutoPolicy, paused bool, c AutoCandidate) string {
	switch {
	case !p.Enabled:
		return AutoDisabled
	case paused:
		return AutoPaused
	case !slices.Contains(p.Kinds, string(c.Kind)):
		return AutoKind
	case !AgreeEnough(c.Confidence, p.MinAgree):
		return AutoAgree
	case strings.Contains(c.Title, "要確認"):
		return AutoAmbiguous
	case c.Kind == KindExample && DryRunStateOf(c.DryRun) != DryRunPass:
		return AutoDryRun
	case p.RequireReverse && c.Reverse != "match":
		return AutoReverse
	}
	return AutoOK
}

// AutoEvidenceState は自動承認の証拠ファイルの状態。nil はファイルが無い。
type AutoEvidenceState struct {
	SHAMatch bool   `json:"sha_match"`
	Verdict  string `json:"verdict"`
}

// 証拠の照合の理由（「証拠の照合」）。
const (
	EvidenceMissing  = "自動承認の証拠が無い"
	EvidenceChanged  = "証拠が承認時と違う"
	EvidenceNotOK    = "証拠の判定が ok でない"
	AutoApproverName = "auto"
)

// AutoEvidenceCheck は承認済みとみなすかと、みなさない理由。approved.by が auto でなければ証拠を見ない。
func AutoEvidenceCheck(by string, ev *AutoEvidenceState) (approved bool, detail string) {
	switch {
	case by != AutoApproverName:
		return true, ""
	case ev == nil:
		return false, EvidenceMissing
	case !ev.SHAMatch:
		return false, EvidenceChanged
	case ev.Verdict != AutoOK:
		return false, EvidenceNotOK
	}
	return true, ""
}

// AutoBreakerTripped は、古い順に並べた自動承認の差し戻し（reset_at より前の差し戻しは false にしたもの）のうち、
// 新しい window 件の差し戻しが max_revoked 以上かを返す。
func AutoBreakerTripped(window, maxRevoked int, revoked []bool) bool {
	recent := revoked[max(len(revoked)-window, 0):]
	n := 0
	for _, r := range recent {
		if r {
			n++
		}
	}
	return n >= maxRevoked
}
