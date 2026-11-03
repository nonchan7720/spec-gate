package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/services"
)

func printErr(err error) { fmt.Fprintln(os.Stderr, "error:", err) }

var mark = map[domain.Status]string{
	domain.StatusPass:       "✓",
	domain.StatusFail:       "✗",
	domain.StatusStale:      "⟳",
	domain.StatusUnapproved: "?",
	domain.StatusUnverified: "–",
}

// writeReport は人間向けと Stop hook（エージェント向け）の両方で使う。
// hook のときは pass を省き、エージェントが直すべきものだけを出す。
func writeReport(w io.Writer, rep services.CheckReport, forHook bool) {
	if len(rep.Claims) > 0 {
		_, _ = fmt.Fprintln(w, "## claims")
		for _, r := range rep.Claims {
			writeResult(w, r, forHook)
		}
	}
	for _, p := range rep.Plans {
		_, _ = fmt.Fprintf(w, "## plan: %s (%s)\n", p.Title, p.Path)
		for _, r := range p.Items {
			writeResult(w, r, forHook)
		}
	}

	all := append(domain.Results{}, rep.Claims...)
	for _, p := range rep.Plans {
		all = append(all, p.Items...)
	}
	_, _ = fmt.Fprintf(w, "\n%d pass, %d fail, %d stale, %d unapproved, %d unverified",
		all.Count(domain.StatusPass), all.Count(domain.StatusFail), all.Count(domain.StatusStale),
		all.Count(domain.StatusUnapproved), all.Count(domain.StatusUnverified))
	if rep.Skipped > 0 {
		_, _ = fmt.Fprintf(w, " (%d claims skipped: 変更と無関係)", rep.Skipped)
	}
	_, _ = fmt.Fprintln(w)
}

func writeResult(w io.Writer, r domain.Result, forHook bool) {
	// hook ではエージェントが直せるものだけ見せる（unverified は人間のレビュー対象）
	if forHook && (r.Status == domain.StatusPass || r.Status == domain.StatusUnverified) {
		return
	}
	spec := ""
	if r.Spec != "" {
		spec = "  [" + r.Spec + "]"
	}
	_, _ = fmt.Fprintf(w, "%s %-14s %s  %s%s\n", mark[r.Status], r.ID, r.Status, r.Title, spec)
	if r.Detail != "" && (forHook || r.Status != domain.StatusPass) {
		for l := range strings.SplitSeq(r.Detail, "\n") {
			_, _ = fmt.Fprintf(w, "      %s\n", l)
		}
	}
}
