package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/services"
	"github.com/spf13/cobra"
)

func newStatusCmd(a **app) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "claim とプランの一覧（判定は走らせない）",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rows, plans, err := (*a).status.List()
			if err != nil {
				return err
			}
			fmt.Println("## claims")
			for _, r := range rows {
				ap := "unapproved"
				if r.Approved {
					ap = "approved " + r.Claim.Approved.At[:10]
				}
				fmt.Printf("%-12s %-10s %-22s %s\n      %s\n", r.Claim.ID, r.Claim.Kind, ap, r.Claim.Title, r.Claim.Source.Ref)
			}
			if len(plans) > 0 {
				fmt.Println("\n## plans")
				for _, p := range plans {
					verified := 0
					for _, it := range p.Items {
						if it.Verify != "" {
							verified++
						}
					}
					fmt.Printf("%-8s %s  (%d items, %d with verify)  %s\n", p.Status, p.Path, len(p.Items), verified, p.Title)
				}
			}
			return nil
		},
	}
}

func newLintCmd(a **app) *cobra.Command {
	return &cobra.Command{
		Use:   "lint",
		Short: "claim の内部整合を検査する（examples vs properties、holdout の形、claim 間の矛盾）",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// holdout は形だけを見る。holdout の指摘は番号だけで、値は出さない
			issues, n, err := (*a).lint.Run()
			if err != nil {
				return err
			}
			if len(issues) == 0 {
				fmt.Printf("ok: %d claims\n", n)
				return nil
			}
			for _, is := range issues {
				fmt.Printf("✗ %-12s %s\n", is.ClaimID, is.Message)
			}
			os.Exit(1)
			return nil
		},
	}
}

func newApproveCmd(a **app) *cobra.Command {
	var by string
	var bulk, yes bool
	var minAgree int
	var kinds []string
	cmd := &cobra.Command{
		Use:   "approve <claim-id>... | approve --pending [--min-agree N] [--kind K] [--yes]",
		Short: "claim を承認し、仕様の現在のハッシュを記録する（lint 失敗時・試し実行が error のときは拒否）",
		Long: `approve <claim-id>... は claims/<領域>/ に置いた claim を承認する。

approve --pending は claims/_pending/ のまとめ承認の対象（既定: 合意が全員一致で、kind: example なら試し実行が pass）を
承認しやすい順に一覧する。--yes を付けたときだけ、1 件ずつ承認画面の承認と同じ処理で承認する
（example は承認の前に試し実行し直し、pass でなければ承認しない）。拒否が 1 件でもあれば exit 1。`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if by == "" {
				by = currentUser()
			}
			if bulk {
				if len(args) > 0 {
					return errors.New("--pending と claim ID は同時に指定できない")
				}
				ks := make([]domain.ClaimKind, 0, len(kinds))
				for _, k := range kinds {
					ks = append(ks, domain.ClaimKind(k))
				}
				failed, err := runBulkApprove(cmd.Context(), newReviewService(*a), cmd.OutOrStdout(), minAgree, ks, yes, by)
				if err != nil {
					return err
				}
				if failed {
					os.Exit(1)
				}
				return nil
			}
			if len(args) == 0 {
				return errors.New("承認する claim ID を指定する（まとめ承認は --pending）")
			}
			for _, id := range args {
				res, err := (*a).approve.Approve(cmd.Context(), id, by)
				if err != nil {
					return fmt.Errorf("%s: %w", id, err)
				}
				fmt.Printf("✓ %s approved by %s\n", id, by)
				if res.Excerpt != "" {
					fmt.Printf("  --- 承認した仕様 (%s) ---\n  %s\n", res.Claim.Source.Ref, strings.ReplaceAll(res.Excerpt, "\n", "\n  "))
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&by, "by", "", "承認者（既定: OS ユーザー名）")
	cmd.Flags().BoolVar(&bulk, "pending", false, "claims/_pending/ のまとめ承認の対象を一覧する（--yes で承認）")
	cmd.Flags().BoolVar(&yes, "yes", false, "--pending の対象を実際に承認する")
	cmd.Flags().IntVar(&minAgree, "min-agree", 0, "--pending: 合意数の下限（既定 0 = 全員一致）")
	cmd.Flags().StringSliceVar(&kinds, "kind", nil, "--pending: kind で絞る（繰り返し可）")
	return cmd
}

// runBulkApprove はまとめ承認の対象を一覧し、yes なら承認する。拒否が 1 件でもあれば failed。
func runBulkApprove(ctx context.Context, svc *services.ReviewService, w io.Writer, minAgree int, kinds []domain.ClaimKind, yes bool, by string) (failed bool, err error) {
	targets, err := svc.BulkTargets(minAgree, kinds)
	if err != nil {
		return false, err
	}
	if len(targets) == 0 {
		_, _ = fmt.Fprintln(w, "まとめ承認の対象はありません")
		return false, nil
	}
	names := make([]string, 0, len(targets))
	for _, p := range targets {
		dr := p.DryRun
		if dr == "" {
			dr = "-"
		}
		_, _ = fmt.Fprintf(w, "  %-5s %-9s %-6s %-16s %s\n", p.Confidence, p.Claim.Kind, dr, p.Claim.ID, p.Claim.Title)
		names = append(names, p.Name)
	}
	if !yes {
		_, _ = fmt.Fprintf(w, "\n%d 件。内容を確かめたら --yes を付けて実行すると承認する（example は承認の前に試し実行し直す）。\n", len(targets))
		return false, nil
	}
	_, _ = fmt.Fprintln(w)
	ok := 0
	for _, o := range svc.BulkApprove(ctx, names, by, true) {
		if o.Error != "" {
			failed = true
			_, _ = fmt.Fprintf(w, "✗ %s: %s\n", o.Name, o.Error)
			continue
		}
		ok++
		_, _ = fmt.Fprintf(w, "✓ %s approved by %s\n", o.ID, by)
	}
	_, _ = fmt.Fprintf(w, "\n%d / %d 件を承認した\n", ok, len(names))
	return failed, nil
}

func newCoverageCmd(a **app) *cobra.Command {
	return &cobra.Command{
		Use:   "coverage",
		Short: "sources.yaml の仕様のうち claim に紐づかない箇所を列挙する",
		Long: `sources.yaml の仕様を単位（見出し・エッジ）に分け、claim に紐づかないものを ✗ で出す（あれば exit 1）。
承認 UI で「曖昧」と記録された claim（claims/_ambiguous/）が参照する単位は「要確認」として ? で出す。
要確認は仕様が決まっていないことの記録なので、exit code には数えない。`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rep, err := (*a).coverage.Report(cmd.Context())
			if err != nil {
				return err
			}
			if writeCoverage(os.Stdout, rep) > 0 {
				os.Exit(1)
			}
			return nil
		},
	}
}

// writeCoverage は coverage のレポートを出し、claim に紐づかない単位の数を返す。
func writeCoverage(w io.Writer, rep services.CoverageReport) int {
	_, _ = fmt.Fprintf(w, "## claims by kind (%d total)\n", rep.Total)
	for _, k := range services.Kinds {
		_, _ = fmt.Fprintf(w, "  %-10s %d\n", k, rep.ByKind[k])
	}
	uncovered, review := 0, 0
	for _, e := range rep.Entries {
		_, _ = fmt.Fprintf(w, "\n## %s\n", e.Source)
		if e.Error != "" {
			_, _ = fmt.Fprintf(w, "  ! %s\n", e.Error)
			continue
		}
		for _, u := range e.Covered {
			_, _ = fmt.Fprintf(w, "  ✓ %s\n", unit(u))
		}
		for _, u := range e.NonNormative {
			_, _ = fmt.Fprintf(w, "  · %s (non-normative)\n", unit(u))
		}
		for _, n := range e.NeedsReview {
			_, _ = fmt.Fprintf(w, "  ? %s  ← 要確認（曖昧: %s）\n", unit(n.Unit), strings.Join(n.Reasons, " / "))
			review++
		}
		for _, u := range e.Uncovered {
			_, _ = fmt.Fprintf(w, "  ✗ %s  ← claim なし\n", unit(u))
			uncovered++
		}
	}
	if review > 0 {
		_, _ = fmt.Fprintf(w, "\n%d units need review (要確認: 仕様が決まっていない)\n", review)
	}
	if uncovered > 0 {
		_, _ = fmt.Fprintf(w, "\n%d units uncovered\n", uncovered)
	}
	return uncovered
}

func newPlanCmd(a **app) *cobra.Command {
	var by string
	plan := &cobra.Command{Use: "plan", Short: "プランファイルの操作"}
	freeze := &cobra.Command{
		Use:   "freeze <plans/xxx.md>",
		Short: "verify 行を凍結する。以後 verify を書き換えると check が stale になる",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if by == "" {
				by = currentUser()
			}
			// mutation score の計算中に中断されても、変異を入れたファイルを戻してから終わる
			ctx, stop := interruptible(cmd.Context())
			defer stop()
			p, res, err := (*a).plan.Freeze(ctx, args[0], by)
			if err != nil {
				return err
			}
			fmt.Printf("✓ %s frozen (%d items)\n", p.Path, len(p.Items))
			if res != nil {
				fmt.Printf("  mutation score %.2f（%d/%d 件を検出、閾値 %v）\n", res.Score(), res.Killed, res.Total, *p.Mutation)
			}
			for _, it := range p.Items {
				fmt.Printf("  L%-3d %s\n       verify: %s\n", it.Line, it.Text, it.Verify)
			}
			return nil
		},
	}
	freeze.Flags().StringVar(&by, "by", "", "凍結者（既定: OS ユーザー名）")
	check := &cobra.Command{
		Use:   "check <plans/xxx.md>...",
		Short: "凍結済みプランの verify 行が凍結時から変わっていないか（変わっていれば exit 1）",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			failed := false
			for _, path := range args {
				if err := (*a).plan.CheckFrozen(path); err != nil {
					fmt.Fprintln(os.Stderr, "✗", err)
					failed = true
				}
			}
			if failed {
				os.Exit(1)
			}
			return nil
		},
	}
	plan.AddCommand(freeze, check, newPlanMutateCmd(a))
	return plan
}

func currentUser() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return "unknown"
}

func unit(u string) string {
	if _, after, ok := strings.Cut(u, "#"); ok {
		return after
	}
	return u
}
