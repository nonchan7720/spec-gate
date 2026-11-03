package cli

import (
	"fmt"
	"os"
	"os/user"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
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
	cmd := &cobra.Command{
		Use:   "approve <claim-id>...",
		Short: "claim を承認し、仕様の現在のハッシュを記録する（lint 失敗時は拒否）",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if by == "" {
				by = currentUser()
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
	return cmd
}

func newCoverageCmd(a **app) *cobra.Command {
	return &cobra.Command{
		Use:   "coverage",
		Short: "sources.yaml の仕様のうち claim に紐づかない箇所を列挙する",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rep, err := (*a).coverage.Report(cmd.Context())
			if err != nil {
				return err
			}
			fmt.Printf("## claims by kind (%d total)\n", rep.Total)
			for _, k := range []domain.ClaimKind{domain.KindExample, domain.KindStructure, domain.KindUI, domain.KindManual} {
				fmt.Printf("  %-10s %d\n", k, rep.ByKind[k])
			}
			uncovered := 0
			for _, e := range rep.Entries {
				fmt.Printf("\n## %s\n", e.Source)
				if e.Error != "" {
					fmt.Printf("  ! %s\n", e.Error)
					continue
				}
				for _, u := range e.Covered {
					fmt.Printf("  ✓ %s\n", unit(u))
				}
				for _, u := range e.NonNormative {
					fmt.Printf("  · %s (non-normative)\n", unit(u))
				}
				for _, u := range e.Uncovered {
					fmt.Printf("  ✗ %s  ← claim なし\n", unit(u))
					uncovered++
				}
			}
			if uncovered > 0 {
				fmt.Printf("\n%d units uncovered\n", uncovered)
				os.Exit(1)
			}
			return nil
		},
	}
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
