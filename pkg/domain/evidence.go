package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Evidence は自動承認の証拠（docs/spec/auto-approval.md「自動承認」）。判定のたびに .spec-gate/evidence/<ID>.json に書く。
type Evidence struct {
	ID           string        `json:"id"`
	Ref          string        `json:"ref"`
	SourceHash   string        `json:"source_hash"`
	PolicySHA256 string        `json:"policy_sha256"`
	Runs         []EvidenceRun `json:"runs"`
	Confidence   string        `json:"confidence"`
	Reverse      string        `json:"reverse"`
	Differences  []string      `json:"differences,omitempty"`
	DryRun       string        `json:"dry_run"`
	Verdict      string        `json:"verdict"`
	At           string        `json:"at"`
}

// EvidenceRun は抽出 1 回分。回答そのものは残さず sha256 だけ残す（途中の回答はファイルに書かない）。
type EvidenceRun struct {
	Command      string `json:"command"`
	AnswerSHA256 string `json:"answer_sha256"`
}

// SHA256Ref は中身の sha256 を `sha256:<16 進>` で返す。
func SHA256Ref(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// EvidenceStateOf は証拠ファイルの中身 raw を、承認に記録した sha256（approvedSHA）と照らして状態にする。
// 読めない証拠は判定が空（ok でない）として扱う。
func EvidenceStateOf(raw []byte, approvedSHA string) *AutoEvidenceState {
	var e Evidence
	_ = json.Unmarshal(raw, &e)
	return &AutoEvidenceState{SHAMatch: SHA256Ref(raw) == approvedSHA, Verdict: e.Verdict}
}
