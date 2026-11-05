package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/nonchan7720/spec-gate/pkg/infrastructure/claimrepo"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/evidence"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/tracelog"
	"github.com/nonchan7720/spec-gate/pkg/services"
	"github.com/spf13/cobra"
)

func newAuditService(a *app) *services.AuditService {
	log := tracelog.NewDecisions(a.root)
	return &services.AuditService{Claims: a.claims, Revoker: claimrepo.New(a.root), Log: log, Decisions: log, Evidence: evidence.New(a.root), Clock: utcClock{}}
}

func newAuditCmd(a **app) *cobra.Command {
	var since string
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "自動承認した claim を新しい順に一覧する（見張り。読み取りだけ）",
		Long: `判定の記録（.spec-gate/review-decisions.jsonl）から自動承認（承認者 auto）を新しい順に出す。
ID・title・仕様の場所・合意数・逆変換・試し実行・時刻・証拠のパス・差し戻し済みか。
--since は RFC 3339 の時刻か 24h のような期間。docs/spec/auto-approval.md「見張りと差し戻し」。`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cutoff, err := parseSince(since, time.Now())
			if err != nil {
				return err
			}
			items, err := newAuditService(*a).List(cutoff)
			if err != nil {
				return err
			}
			writeAudit(cmd.OutOrStdout(), items)
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "この時刻（RFC 3339）か、この期間（24h など）より前の自動承認は出さない")
	return cmd
}

// parseSince は --since を RFC 3339（UTC）の時刻にする。空なら空。
func parseSince(s string, now time.Time) (string, error) {
	if s == "" {
		return "", nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format(time.RFC3339), nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return "", fmt.Errorf("--since は RFC 3339 の時刻か 24h のような期間: %q", s)
	}
	return now.Add(-d).UTC().Format(time.RFC3339), nil
}

func writeAudit(w io.Writer, items []services.AuditItem) {
	if len(items) == 0 {
		_, _ = fmt.Fprintln(w, "自動承認はありません")
		return
	}
	for _, it := range items {
		mark := "✓"
		if it.Revoked {
			mark = "↩"
		}
		title := it.Title
		if title == "" {
			title = "（claim が見当たらない）"
		}
		_, _ = fmt.Fprintf(w, "%s %s  %-12s %s\n", mark, it.Time, it.ID, title)
		_, _ = fmt.Fprintf(w, "    %s  合意 %s  逆変換 %s  試し実行 %s  証拠 %s\n", orDash(it.Ref), orDash(it.Confidence), orDash(it.Reverse), orDash(it.DryRun), orDash(it.Evidence))
	}
	_, _ = fmt.Fprintf(w, "\n%d 件（↩ は差し戻し済み）。差し戻すには人間が spec-gate revoke <ID> --reason <理由>\n", len(items))
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func newRevokeCmd(a **app) *cobra.Command {
	var reason, by string
	cmd := &cobra.Command{
		Use:   "revoke <claim-id>",
		Short: "承認済みの claim の承認を取り消し、claims/_pending/ に戻す（人間だけ）",
		Long: `approved を消して claims/_pending/<ID>.yaml に戻し、先頭に「# revoked: <理由>」を書く。理由は必須。
自動承認でない claim も取り消せる。判定 revoke として .spec-gate/review-decisions.jsonl に記録し、
差し戻しはブレーカー（docs/spec/auto-approval.md「ブレーカー」）に数えられる。エージェントは実行できない（spec-gate guard が拒否する）。`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(reason) == "" {
				return errors.New("--reason（取り消す理由）は必須")
			}
			if by == "" {
				by = currentUser()
			}
			path, err := newAuditService(*a).Revoke(args[0], reason, by)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "↩ %s の承認を取り消して %s に戻した\n", args[0], path)
			return nil
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "取り消す理由（必須）")
	cmd.Flags().StringVar(&by, "by", "", "取り消した人（既定: OS ユーザー名）")
	return cmd
}
