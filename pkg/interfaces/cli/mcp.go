package cli

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nonchan7720/spec-gate/pkg/interfaces/mcpapp"
	"github.com/nonchan7720/spec-gate/pkg/version"
	"github.com/spf13/cobra"
)

func newMCPCmd(a **app) *cobra.Command {
	var by string
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "承認 UI（MCP Apps）の MCP サーバを stdio で起動する（.mcp.json から起動される）",
		Long: `claims/_pending/ の承認待ち候補を、リモートセッション（claude.ai/code・スマホの Claude アプリ）でも
タップで承認・却下・曖昧・まとめ承認できる画面（MCP Apps）として出す MCP サーバ。

エージェントが呼べるのは画面を開く review だけ。判断を反映する review_decide / review_bulk_approve は
画面からだけ呼べる（_meta.ui.visibility: ["app"]）。反映は承認画面と同じ処理で、見せた内容の sha256 が
今の候補と一致するときだけ行い、.spec-gate/review-decisions.jsonl に記録する。`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if by == "" {
				by = currentUser()
			}
			s := mcpapp.NewServer(newRemoteReview(*a), by, version.Version)
			return s.Run(cmd.Context(), &mcp.StdioTransport{})
		},
	}
	cmd.Flags().StringVar(&by, "by", "", "承認者（既定: OS ユーザー名）")
	return cmd
}
