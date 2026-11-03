package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/tracelog"
	"github.com/nonchan7720/spec-gate/pkg/services"
	"github.com/spf13/cobra"
)

// recordHookTrace は check --hook の claim の判定を通り方のログに追記する。
// 記録は人間が後から見るための補助なので、書けなくても hook の結果（エージェントを止めるか）は変えない。
func recordHookTrace(tr *services.TraceService, rep services.CheckReport) {
	if tr == nil || len(rep.Claims) == 0 {
		return
	}
	_ = tr.Record(rep.Claims)
}

func newTraceCmd(a **app) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "trace",
		Short: "claim ごとに、最後に pass になったときの直前の連続 fail 回数と変更行数を出す（通り方のログ）",
		Long: `check --hook のたびに ` + tracelog.RelPath + ` に追記した判定履歴を claim ごとにまとめる。

  fails  最後に pass になった直前の連続 fail 回数
  lines  その pass の時点の変更行数（git diff --numstat HEAD の追加行と削除行の合計）

何度も fail した末に大きな変更で pass した claim は、無理に通していないかを人間が確かめる目安になる。`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			traces, err := (*a).trace.Summary()
			if err != nil {
				return err
			}
			return writeTrace(os.Stdout, traces, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "JSON で出す")
	return cmd
}

type jsonTrace struct {
	ID   string               `json:"id"`
	Runs int                  `json:"runs"`
	Last domain.Status        `json:"last"`
	Pass *domain.TraceSummary `json:"pass"`
}

// writeTrace は claim ごとの通り方をテキストか JSON で出す。
func writeTrace(w io.Writer, traces []services.ClaimTrace, asJSON bool) error {
	if asJSON {
		out := make([]jsonTrace, 0, len(traces))
		for _, t := range traces {
			out = append(out, jsonTrace{ID: t.ID, Runs: t.Runs, Last: t.Last, Pass: t.Pass})
		}
		return writeJSON(w, out)
	}
	if len(traces) == 0 {
		_, err := fmt.Fprintf(w, "記録なし（%s は check --hook のたびに追記される）\n", tracelog.RelPath)
		return err
	}
	if _, err := fmt.Fprintf(w, "%-14s %5s %7s  %s\n", "CLAIM", "FAILS", "LINES", "最後の判定 / 記録回数"); err != nil {
		return err
	}
	for _, t := range traces {
		var err error
		if t.Pass == nil {
			_, err = fmt.Fprintf(w, "%-14s %5s %7s  %s / %d 回（pass していない）\n", t.ID, "-", "-", t.Last, t.Runs)
		} else {
			_, err = fmt.Fprintf(w, "%-14s %5d %7d  %s / %d 回\n", t.ID, t.Pass.FailsBeforePass, t.Pass.LinesAtPass, t.Last, t.Runs)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
