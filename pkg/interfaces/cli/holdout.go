package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

func newHoldoutCmd(a **app) *cobra.Command {
	holdout := &cobra.Command{Use: "holdout", Short: "holdout（エージェントに見せない入出力例）の操作。人間だけが使う"}
	var in, out string
	add := &cobra.Command{
		Use:   "add <claim-id> --in <json> --out <json>",
		Short: "holdout に 1 件足す（期待値は正規化 JSON の SHA-256 だけを保存する）",
		Long: `holdout/<claim-id>.yaml の examples の末尾に in と out_sha256 を足す（ファイルが無ければ作る）。
out の平文はどこにも保存も表示もしない。holdout を書くのは人間だけで、エージェントからの実行は hook-gate で拒否する。`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHoldoutAdd(os.Stdout, *a, args[0], in, out)
		},
	}
	add.Flags().StringVar(&in, "in", "", "binding に渡す入力（JSON）")
	add.Flags().StringVar(&out, "out", "", "期待する出力（JSON）。ハッシュにして保存し、平文は残さない")
	_ = add.MarkFlagRequired("in")
	_ = add.MarkFlagRequired("out")
	holdout.AddCommand(add)
	return holdout
}

// runHoldoutAdd は --in / --out の JSON を読み、holdout に 1 件足して書いたファイルを w に出す。
// out の値はエラーにも出力にも出さない。
func runHoldoutAdd(w io.Writer, a *app, claimID, inJSON, outJSON string) error {
	var in, out any
	if err := json.Unmarshal([]byte(inJSON), &in); err != nil {
		return fmt.Errorf("--in が JSON ではない: %w", err)
	}
	if err := json.Unmarshal([]byte(outJSON), &out); err != nil {
		return errors.New("--out が JSON ではない（値は表示しない）")
	}
	path, err := a.holdoutAdd.Add(claimID, in, out)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "✓ %s に holdout を 1 件足した（期待値は out_sha256 だけを保存）\n", path)
	return nil
}
