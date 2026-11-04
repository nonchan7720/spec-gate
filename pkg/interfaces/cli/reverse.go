package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/reverser"
	"github.com/nonchan7720/spec-gate/pkg/services"
	"github.com/spf13/cobra"
)

const reverseHelp = `reverse は LLM を直接呼ばない。次のどちらかで実行する。

1. サブエージェントに委譲する（推奨。手順は .claude/skills/spec-gate-reverse/SKILL.md）
     spec-gate reverse [<claim-id>...] --emit
       claim ごとに再生成のプロンプト（元の仕様は含めない）を .spec-gate-llm/ に書き出し、そのパスを表示する
     プロンプト 1 つにつき新しい spec-gate-llm-worker サブエージェントを 1 体、並列に起動し、
       「Read <プロンプトのパス> and follow it.」とだけ渡す
     spec-gate reverse --collect
       再生成の回答がそろえば比較のプロンプトを書き出す → 同じように新しいサブエージェントで答えさせる
     spec-gate reverse --collect
       比較の回答がそろえばレポートを出す

2. LLM コマンドを直接呼ぶ
     SPEC_GATE_REVERSE_CMD='claude -p' spec-gate reverse [<claim-id>...]`

func newReverseCmd(a **app) *cobra.Command {
	var strict bool
	var llm llmFlags
	cmd := &cobra.Command{
		Use:   "reverse [<claim-id>...]",
		Short: "claim だけから仕様文を再生成し、元の仕様と食い違いがないかを LLM で突き合わせる（逆変換）",
		Long: `claim の内容だけ（元の仕様は見せない）から仕様文を書き起こし、元の仕様の該当箇所と比べる。
claim にあって仕様に無い主張、仕様にあって claim に無い規範、矛盾を列挙する。承認前のレビュー補助。
claim-id を省略すると全 claim（_pending は対象外）。figma / image の claim と、仕様が読めない claim は skipped。

既定では常に exit 0。--strict を付けると mismatch / error があれば exit 1。

例:
  spec-gate reverse ROUND-001 --emit       # 再生成のプロンプトを書き出す
  spec-gate reverse --collect              # 再生成の回答 → 比較のプロンプトを書き出す
  spec-gate reverse --collect --strict     # 比較の回答 → レポート
  SPEC_GATE_REVERSE_CMD='claude -p' spec-gate reverse

spec-gate 自身は LLM を呼ばない。--emit / --collect でサブエージェントに委譲するか、
環境変数 SPEC_GATE_REVERSE_CMD に LLM コマンド（stdin に prompt、stdout に応答）を指定する。
作業ディレクトリは .spec-gate-llm/（レポートを出したら消す）。`,
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := llm.mode("SPEC_GATE_REVERSE_CMD", reverseHelp)
			if err != nil {
				return err
			}
			svc := newReverseService(*a)
			var rep services.ReverseReport
			switch mode {
			case modeEmit, modeCollect:
				var d services.ReverseDelegation
				if mode == modeEmit {
					d, err = svc.Emit(cmd.Context(), args)
				} else {
					d, err = svc.Collect(cmd.Context(), args)
				}
				if err != nil {
					return err
				}
				if d.Phase != services.ReversePhaseDone {
					writePrompts(cmd.OutOrStdout(), cmd.ErrOrStderr(), d.Prompts, "spec-gate reverse --collect を実行する。")
					return nil
				}
				rep = *d.Report
			default:
				if rep, err = svc.Reverse(cmd.Context(), args); err != nil {
					return err
				}
			}
			writeReverseReport(cmd.OutOrStdout(), rep)
			if rep.Failed(strict) {
				os.Exit(1)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&strict, "strict", false, "mismatch / error があれば exit 1")
	llm.bind(cmd,
		"LLM を呼ばず、claim ごとの再生成のプロンプトを .spec-gate-llm/ に書き出してパスを表示する",
		"サブエージェントの回答を読み、次の段階のプロンプトかレポートを出す")
	return cmd
}

// newReverseService は reverse の依存を組み立てる。adapter は newApp と共有する。
// figma / image はテキストを持たないので service 側で skipped になる。
// LLM を直接呼ぶのは環境変数でコマンドを指定したときだけ。
func newReverseService(a *app) *services.ReverseService {
	svc := &services.ReverseService{
		Claims:   a.claims,
		Adapters: a.adapters,
		Prompter: reverser.Prompter{},
		Work:     a.work,
	}
	if command := os.Getenv("SPEC_GATE_REVERSE_CMD"); command != "" {
		svc.Reverser = reverser.Command{Command: command, Dir: a.root, Timeout: 5 * time.Minute}
	}
	return svc
}

var reverseMark = map[domain.ReverseStatus]string{
	domain.ReverseMatch:    "✓",
	domain.ReverseMismatch: "✗",
	domain.ReverseSkipped:  "–",
	domain.ReverseError:    "!",
}

func writeReverseReport(w io.Writer, rep services.ReverseReport) {
	for _, r := range rep.Results {
		spec := ""
		if r.Spec != "" {
			spec = "  [" + r.Spec + "]"
		}
		_, _ = fmt.Fprintf(w, "%s %-12s %-8s %s%s\n", reverseMark[r.Status], r.ID, r.Status, r.Title, spec)
		if r.Detail != "" {
			_, _ = fmt.Fprintf(w, "    %s\n", indent(r.Detail, "    "))
		}
		if r.Restated != "" {
			_, _ = fmt.Fprintf(w, "    再生成:\n      %s\n", indent(r.Restated, "      "))
		}
		if len(r.Differences) > 0 {
			_, _ = fmt.Fprintln(w, "    食い違い:")
			for _, d := range r.Differences {
				_, _ = fmt.Fprintf(w, "      - %s\n", indent(d, "        "))
			}
		}
	}
	_, _ = fmt.Fprintf(w, "\n%d match, %d mismatch, %d skipped, %d error\n",
		rep.Count(domain.ReverseMatch), rep.Count(domain.ReverseMismatch),
		rep.Count(domain.ReverseSkipped), rep.Count(domain.ReverseError))
}

// indent は 2 行目以降に prefix を付ける。
func indent(s, prefix string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), "\n", "\n"+prefix)
}
