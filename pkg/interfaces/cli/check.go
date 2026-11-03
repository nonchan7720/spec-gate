package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/services"
	"github.com/spf13/cobra"
)

func newCheckCmd(a **app) *cobra.Command {
	var (
		only    string
		changed bool
		ui      bool
		noPlans bool
		hook    bool
	)
	cmd := &cobra.Command{
		Use:   "check",
		Short: "claim とプランを判定する（exit 1 = blocking な失敗あり）",
		Long: `承認 → 仕様ドリフト → 種類別チェック → アクティブなプランの verify の順に判定する。

  --changed  git の変更ファイルに関係する claim だけを対象にする
  --only     claim ID をカンマ区切りで指定（プランは見ない）
  --ui       Playwright を実行して strings を検証する（アプリ/Storybook を起動しておくこと）
  --hook     Claude Code の Stop hook 用。失敗時は stderr にレポートを出し exit 2 でエージェントを止める`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			req := services.CheckRequest{Changed: changed, UI: ui, NoPlans: noPlans}
			if only != "" {
				req.Only = strings.Split(only, ",")
			}
			rep, err := (*a).check.Run(cmd.Context(), req)
			if err != nil {
				if hook {
					// ハーネス自体の故障でエージェントを止め続けるのは避ける
					fmt.Fprintln(os.Stderr, "spec-gate 内部エラー（判定スキップ）:", err)
					return nil
				}
				return err
			}
			if hook {
				runHook(rep)
				return nil
			}
			writeReport(os.Stdout, rep, false)
			if rep.HasBlocking() {
				os.Exit(1)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&only, "only", "", "claim ID をカンマ区切りで")
	cmd.Flags().BoolVar(&changed, "changed", false, "変更ファイルに関係する claim だけ")
	cmd.Flags().BoolVar(&ui, "ui", false, "Playwright で strings を検証")
	cmd.Flags().BoolVar(&noPlans, "no-plans", false, "プランを判定しない")
	cmd.Flags().BoolVar(&hook, "hook", false, "Claude Code Stop hook モード")
	return cmd
}

// hookResult は Claude Code hook 規約での終了コードとエージェント向けメッセージを返す。
// exit 0 → 続行、exit 2 → メッセージをエージェントに返して止める。
// ループ防止（連続失敗で諦める）は feedback-gate の gate 側が持つので、ここでは常に止める。
func hookResult(rep services.CheckReport) (int, string) {
	if !rep.HasBlocking() {
		return 0, ""
	}
	var b strings.Builder
	b.WriteString("spec-gate: 仕様準拠チェックに失敗。以下を直してから終了すること。\n")
	b.WriteString("claims/ と plans の verify: 行は編集禁止。コード側を仕様に合わせる。\n")
	b.WriteString("claim 自体が間違っていると考える場合は、claims/_pending/ に修正案を置き、根拠をユーザーに伝えて止まる。\n\n")
	writeReport(&b, rep, true)
	return 2, b.String()
}

func runHook(rep services.CheckReport) {
	code, msg := hookResult(rep)
	if code == 0 {
		return
	}
	_, _ = io.WriteString(os.Stderr, msg)
	os.Exit(code)
}
