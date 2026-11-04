package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/claimrepo"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/tracelog"
	"github.com/nonchan7720/spec-gate/pkg/services"
	"github.com/spf13/cobra"
)

// newReviewService は承認画面・まとめ承認・リモート承認が共有する ReviewService を組み立てる。
func newReviewService(a *app) *services.ReviewService {
	return &services.ReviewService{Pending: claimrepo.New(a.root), Adapters: a.adapters, Approver: a.approve}
}

// newRemoteReview はリモート承認のサービスを組み立てる。反映は承認画面と同じ ReviewService を使う。
func newRemoteReview(a *app) *services.RemoteReviewService {
	review := newReviewService(a)
	return &services.RemoteReviewService{Review: review, Log: tracelog.NewDecisions(a.root), Clock: utcClock{}}
}

func newReviewAskCmd(a **app) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "ask",
		Short: "承認待ちの候補を、質問ダイアログ（AskUserQuestion）にそのまま渡せる questions の JSON にする",
		Long: `claims/_pending/ の候補を 1 つ 1 問にして、AskUserQuestion の questions（JSON 配列）を stdout に出す。
選択肢は「承認」「却下」「曖昧」、各選択肢の preview に候補の内容を平易に書く。
回答は PostToolUse hook の ` + "`spec-gate review record`" + ` が受け取って反映する（リモート承認）。
候補が無ければ [] を出し、stderr に案内する。`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runReviewAsk(newRemoteReview(*a), limit, os.Stdout, os.Stderr)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", services.RemoteAskMax, fmt.Sprintf("出す候補の数（1〜%d）", services.RemoteAskMax))
	return cmd
}

func runReviewAsk(svc *services.RemoteReviewService, limit int, stdout, stderr io.Writer) error {
	qs, notes, err := svc.Ask(limit)
	if err != nil {
		return err
	}
	for _, n := range notes {
		_, _ = fmt.Fprintln(stderr, n)
	}
	return writeJSON(stdout, qs)
}

func newReviewRecordCmd(a **app) *cobra.Command {
	var by string
	cmd := &cobra.Command{
		Use:   "record",
		Short: "PostToolUse hook 用: 質問ダイアログの回答を承認・却下・曖昧として反映する（人が直接実行するものではない）",
		Long: `stdin に PostToolUse hook の JSON（tool_name・tool_input.questions・tool_response）を受け、
review ask で出した質問への回答を反映する。表示された質問（preview を含む）が、spec-gate が今その候補から作る質問と
完全に一致するものだけを反映する。反映した判定は ` + tracelog.DecisionsRelPath + ` に追記する。
結果は hookSpecificOutput.additionalContext として stdout に出す。ダイアログの操作を壊さないよう、常に exit 0。`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if by == "" {
				by = currentUser()
			}
			runReviewRecord(cmd.Context(), newRemoteReview(*a), os.Stdin, os.Stdout, os.Stderr, by)
			return nil
		},
	}
	cmd.Flags().StringVar(&by, "by", "", "承認者（既定: OS ユーザー名）")
	return cmd
}

// runReviewRecord は hook の stdin を反映し、結果を Claude 向けの JSON（stdout）と理由（stderr）に出す。失敗しない。
func runReviewRecord(ctx context.Context, svc *services.RemoteReviewService, stdin io.Reader, stdout, stderr io.Writer, by string) {
	raw, err := io.ReadAll(stdin)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "spec-gate review record: stdin を読めない:", err)
		return
	}
	res := svc.Record(ctx, raw, by)
	for _, n := range res.Notes {
		_, _ = fmt.Fprintln(stderr, "spec-gate review record:", n)
	}
	if len(res.Outcomes) == 0 && len(res.Notes) == 0 {
		return
	}
	out := map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName":     "PostToolUse",
		"additionalContext": remoteContext(res),
	}}
	b, err := json.Marshal(out)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintln(stdout, string(b))
}

var verdictText = map[domain.RemoteVerdict]string{
	domain.VerdictApprove:   "承認",
	domain.VerdictReject:    "却下",
	domain.VerdictAmbiguous: "曖昧",
}

// remoteContext は反映結果を Claude に伝える文にする。
func remoteContext(res services.RemoteRecordResult) string {
	var b strings.Builder
	b.WriteString("spec-gate リモート承認の結果（人間がダイアログで選んだもの。エージェントは claims/ を触らずに続けること）:\n")
	for _, o := range res.Outcomes {
		d := o.Decision
		line := fmt.Sprintf("- %s: %s", d.ID, verdictText[d.Verdict])
		if d.Reason != "" {
			line += "（理由: " + d.Reason + "）"
		}
		if o.Error != "" {
			line += " → 反映できなかった: " + o.Error
		} else {
			line += " → 反映した"
		}
		b.WriteString(line + "\n")
	}
	for _, n := range res.Notes {
		b.WriteString("- 反映しなかった: " + n + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
