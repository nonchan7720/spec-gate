package cli

import (
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
		asJSON  bool
	)
	cmd := &cobra.Command{
		Use:   "check",
		Short: "claim とプランを判定する（exit 1 = blocking な失敗あり）",
		Long: `承認 → 仕様ドリフト → 種類別チェック → アクティブなプランの verify の順に判定する。

  --changed  git の変更ファイルに関係する claim だけを対象にする
  --only     claim ID をカンマ区切りで指定（プランは見ない）
  --ui       Playwright を実行して strings を検証する（アプリ/Storybook を起動しておくこと）
  --hook     Claude Code の Stop hook 用。失敗時は stderr にレポートを出し exit 2 でエージェントを止める
  --json     CI 用に判定結果（各 claim の id・kind・status・title・detail・spec と件数）を JSON で出す。exit code は同じ`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			req := services.CheckRequest{Changed: changed, UI: ui, NoPlans: noPlans}
			if only != "" {
				req.Only = strings.Split(only, ",")
			}
			rep, err := (*a).check.Run(cmd.Context(), req)
			if err != nil {
				if hook {
					exitHook(hookError(err))
				}
				return err
			}
			if hook {
				exitHook(hookResult(rep))
				return nil
			}
			code, err := renderCheck(os.Stdout, rep, checkOutput{JSON: asJSON})
			if err != nil {
				return err
			}
			if code != 0 {
				os.Exit(code)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&only, "only", "", "claim ID をカンマ区切りで")
	cmd.Flags().BoolVar(&changed, "changed", false, "変更ファイルに関係する claim だけ")
	cmd.Flags().BoolVar(&ui, "ui", false, "Playwright で strings を検証")
	cmd.Flags().BoolVar(&noPlans, "no-plans", false, "プランを判定しない")
	cmd.Flags().BoolVar(&hook, "hook", false, "Claude Code Stop hook モード")
	cmd.Flags().BoolVar(&asJSON, "json", false, "判定結果を JSON で出す（CI 用）")
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

// hookError はハーネス自体が判定できなかった（claim が読めない等）ときの hook の結果。
// exit 0 で通すと「検証していないのに通った」になるので止める。止め続けるループは
// feedback-gate の gate 側が打ち切る。
func hookError(err error) (int, string) {
	var b strings.Builder
	b.WriteString("spec-gate: 内部エラーで仕様準拠を判定できなかった。原因を直してから終了すること。\n")
	b.WriteString("claims/ と plans の verify: 行が原因ならエージェントは直さず、内容をユーザーに伝えて止まる。\n\n")
	b.WriteString(err.Error())
	b.WriteString("\n")
	return 2, b.String()
}

func exitHook(code int, msg string) {
	if code == 0 {
		return
	}
	_, _ = io.WriteString(os.Stderr, msg)
	os.Exit(code)
}
