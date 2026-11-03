package cli

import (
	"fmt"
	"runtime"

	"github.com/nonchan7720/spec-gate/pkg/version"
	"github.com/spf13/cobra"
)

// versionTemplate は `spec-gate --version` の出力。
func versionTemplate() string {
	return fmt.Sprintf("spec-gate {{.Version}} (commit %s, built %s)\n", version.Commit, version.Date)
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "バージョン・コミット・ビルド日時・Go ツールチェーンを表示する",
		Args:  cobra.NoArgs,
		// リポジトリを読まないので、ルートの --root による組み立てを飛ばす
		PersistentPreRunE: func(*cobra.Command, []string) error { return nil },
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "spec-gate %s\ncommit: %s\nbuilt:  %s\ngo:     %s %s/%s\n",
				version.Version, version.Commit, version.Date, runtime.Version(), runtime.GOOS, runtime.GOARCH)
			return err
		},
	}
}
