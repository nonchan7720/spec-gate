package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/services"
)

// checkOutput は check の出力形式。
type checkOutput struct {
	// JSON は CI 向けに判定結果を JSON で出す
	JSON bool
	// Role が空でなければ、その役割が対応すべき判定だけを出す
	Role domain.Role
}

// renderCheck は判定結果を出力し、check の exit code を返す。
// 出力形式に関わらず exit code は同じ（blocking があれば 1）。
func renderCheck(w io.Writer, rep services.CheckReport, opt checkOutput) (int, error) {
	code := 0
	if rep.HasBlocking() {
		code = 1
	}
	switch {
	case opt.JSON:
		return code, writeJSONReport(w, rep, opt.Role)
	case opt.Role != "":
		writeRoleReport(w, rep, opt.Role)
	default:
		writeReport(w, rep, false)
	}
	return code, nil
}

type jsonResult struct {
	ID     string `json:"id"`
	Kind   string `json:"kind,omitempty"`
	Status string `json:"status"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Spec   string `json:"spec"`
}

type jsonPlan struct {
	Path  string       `json:"path"`
	Title string       `json:"title"`
	Items []jsonResult `json:"items"`
}

type jsonCounts struct {
	Total      int `json:"total"`
	Pass       int `json:"pass"`
	Fail       int `json:"fail"`
	Stale      int `json:"stale"`
	Unapproved int `json:"unapproved"`
	Unverified int `json:"unverified"`
}

type jsonReport struct {
	// Role は --role で絞ったときの役割
	Role   string       `json:"role,omitempty"`
	Claims []jsonResult `json:"claims"`
	Plans  []jsonPlan   `json:"plans"`
	// Counts はテキスト出力の最終行と同じく、claim とプラン項目を合わせた件数（--role の絞り込み前）
	Counts  jsonCounts `json:"counts"`
	Skipped int        `json:"skipped"`
}

func toJSONResults(rs domain.Results) []jsonResult {
	out := make([]jsonResult, 0, len(rs))
	for _, r := range rs {
		out = append(out, jsonResult{ID: r.ID, Kind: string(r.Kind), Status: string(r.Status), Title: r.Title, Detail: r.Detail, Spec: r.Spec})
	}
	return out
}

// allResults は claim とプラン項目をまとめる（件数の集計用）。
func allResults(rep services.CheckReport) domain.Results {
	all := append(domain.Results{}, rep.Claims...)
	for _, p := range rep.Plans {
		all = append(all, p.Items...)
	}
	return all
}

func writeJSONReport(w io.Writer, rep services.CheckReport, role domain.Role) error {
	shown := rep
	if role != "" {
		shown = filterReport(rep, role)
	}
	out := jsonReport{Role: string(role), Claims: toJSONResults(shown.Claims), Plans: make([]jsonPlan, 0, len(shown.Plans)), Skipped: rep.Skipped}
	for _, p := range shown.Plans {
		out.Plans = append(out.Plans, jsonPlan{Path: p.Path, Title: p.Title, Items: toJSONResults(p.Items)})
	}
	all := allResults(rep)
	out.Counts = jsonCounts{
		Total:      len(all),
		Pass:       all.Count(domain.StatusPass),
		Fail:       all.Count(domain.StatusFail),
		Stale:      all.Count(domain.StatusStale),
		Unapproved: all.Count(domain.StatusUnapproved),
		Unverified: all.Count(domain.StatusUnverified),
	}
	return writeJSON(w, out)
}

func writeJSON(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(b))
	return err
}

// writeMetrics は metrics をテキストか JSON で出す。
func writeMetrics(w io.Writer, m services.Metrics, asJSON bool) error {
	if asJSON {
		return writeJSON(w, m)
	}
	if _, err := fmt.Fprintf(w, "## claims by kind (%d total)\n", m.Total); err != nil {
		return err
	}
	for _, k := range m.ByKind {
		if _, err := fmt.Fprintf(w, "  %-10s %3d  %5.1f%%\n", k.Kind, k.Count, k.Ratio*100); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "\nunverified: %d / %d (%.1f%%)\n", m.Unverified, m.Total, m.UnverifiedRate*100)
	return err
}
