package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/services"
	"github.com/spf13/cobra"
)

// interruptible は Ctrl-C / SIGTERM で取り消される ctx を返す。
// プロセスをその場で終わらせず ctx を取り消すので、変異を入れたファイルを defer で戻してから終われる。
func interruptible(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
}

func newPlanMutateCmd(a **app) *cobra.Command {
	var max int
	cmd := &cobra.Command{
		Use:   "mutate <plans/xxx.md>",
		Short: "claims が触るファイルに変異を 1 か所ずつ入れて verify を走らせ、mutation score を出す（ファイルは必ず元に戻す）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := interruptible(cmd.Context())
			defer stop()
			p, err := (*a).plans.Load(args[0])
			if err != nil {
				return err
			}
			res, err := (*a).mutation.Score(ctx, p, max)
			if err != nil {
				return err
			}
			if !printMutation(os.Stdout, p, res) {
				os.Exit(1)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&max, "max", services.DefaultMutationMax, "走らせる変異の上限（0 で無制限）")
	return cmd
}

// printMutation は mutation の結果を出す。プランに閾値があって満たさない（score を出せない場合を含む）なら false。
func printMutation(w io.Writer, p domain.Plan, res domain.MutationResult) bool {
	if res.Total == 0 {
		// 変異が 1 件も作れないときは score を出さない
		_, _ = fmt.Fprintf(w, "%s: claims が触るファイルに変異を 1 件も作れない（score なし）\n", p.Path)
		return p.Mutation == nil
	}
	score := res.Score()
	_, _ = fmt.Fprintf(w, "%s: mutation score %.2f（%d/%d 件を検出", p.Path, score, res.Killed, res.Total)
	if res.Candidates > res.Total {
		_, _ = fmt.Fprintf(w, "、変異 %d 件から --max で %d 件を選んだ", res.Candidates, res.Total)
	}
	_, _ = fmt.Fprintln(w, "）")
	if len(res.Survivors) > 0 {
		_, _ = fmt.Fprintf(w, "検出できなかった変異（verify がすべて通った）:\n%s", services.FormatSurvivors(res.Survivors))
	}
	if p.Mutation != nil {
		if score < *p.Mutation {
			_, _ = fmt.Fprintf(w, "✗ 閾値 %v 未満。このままでは plan freeze できない\n", *p.Mutation)
			return false
		}
		_, _ = fmt.Fprintf(w, "✓ 閾値 %v 以上\n", *p.Mutation)
	}
	return true
}
