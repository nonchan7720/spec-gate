package domain

import "strings"

// Status は 1 つの claim / plan 項目の判定結果。
type Status string

const (
	StatusPass       Status = "pass"
	StatusFail       Status = "fail"
	StatusStale      Status = "stale"      // 仕様が承認時から変わった
	StatusUnapproved Status = "unapproved" // 人間の承認がない
	StatusUnverified Status = "unverified" // 検証手段がない
)

// Blocking は Stop hook でエージェントを止めるべき状態か。
// unverified は「人間が見る」ものなのでエージェントは止めない。
func (s Status) Blocking() bool {
	return s == StatusFail || s == StatusStale || s == StatusUnapproved
}

// Result は判定 1 件。
type Result struct {
	ID string
	// Kind は claim の種類（役割別レポート・metrics 用）。プラン項目では空
	Kind   ClaimKind
	Title  string
	Status Status
	Detail string // 失敗理由・補足。複数行可
	Spec   string // 仕様の該当箇所 file:line
}

// Results は判定の集合。
type Results []Result

func (rs Results) Count(s Status) int {
	n := 0
	for _, r := range rs {
		if r.Status == s {
			n++
		}
	}
	return n
}

func (rs Results) HasBlocking() bool {
	for _, r := range rs {
		if r.Status.Blocking() {
			return true
		}
	}
	return false
}

// Snapshot は仕様ソースのある時点の状態。
type Snapshot struct {
	Hash     string // 正規化後のハッシュ。空なら取得失敗
	Excerpt  string // 人間向け抜粋
	Location string // file:line など
	Skipped  string // 取得できなかった理由
	// Suggestion は参照先の見出しが無いときの再リンク候補（最も近い見出し）。無ければ空。
	Suggestion string
}

// SourceEntry は sources.yaml の 1 行。
type SourceEntry struct {
	Type         SourceType `yaml:"type"`
	Path         string     `yaml:"path,omitempty"`
	Ref          string     `yaml:"ref,omitempty"`
	Label        string     `yaml:"label,omitempty"`
	NonNormative []string   `yaml:"non_normative,omitempty"`
}

// RefOf は claim の source.ref と同じ形式に正規化する。
func (e SourceEntry) RefOf() string {
	if e.Ref != "" {
		return e.Ref
	}
	return e.Path
}

func splitWords(s string) []string { return strings.Fields(s) }
