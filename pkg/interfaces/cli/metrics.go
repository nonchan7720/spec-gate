package cli

import (
	"os"

	"github.com/nonchan7720/spec-gate/pkg/services"
	"github.com/spf13/cobra"
)

func newMetricsCmd(a **app) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "claim の kind 別件数と割合、unverified 率を出す（CI 用。--json で JSON）",
		Long: `全 claim を check と同じ手順で判定し（プランは見ない、--ui なし）、次を出す。

  - kind（example / structure / ui / manual）別の件数と、総数に対する割合
  - unverified 率 = unverified の件数 / claim の総数（claim が 0 件なら 0）

判定の失敗では exit 1 にしない（指標を出すだけ）。失敗で止めたいときは check を使う。`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rep, err := (*a).check.Run(cmd.Context(), services.CheckRequest{NoPlans: true})
			if err != nil {
				return err
			}
			return writeMetrics(os.Stdout, services.ComputeMetrics(rep.Claims), asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "JSON で出す")
	return cmd
}
