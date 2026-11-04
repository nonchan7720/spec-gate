package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/services"
	"github.com/spf13/cobra"
)

// llmMode は LLM 処理の実行のしかた。
type llmMode int

const (
	modeEmit    llmMode = iota // プロンプトを書き出す（サブエージェントに委譲）
	modeCollect                // 回答を取り込む
	modeCommand                // 環境変数のコマンドで LLM を直接呼ぶ
)

// pickLLMMode は --emit / --collect と環境変数から実行のしかたを決める。
// どれも無ければ委譲の手順（help）を案内してエラーにする。spec-gate が意図せず LLM を呼ばないため。
func pickLLMMode(emit, collect bool, envCmd, help string) (llmMode, error) {
	switch {
	case emit && collect:
		return 0, errors.New("--emit と --collect は同時に使えない")
	case emit:
		return modeEmit, nil
	case collect:
		return modeCollect, nil
	case envCmd != "":
		return modeCommand, nil
	}
	return 0, errors.New(help)
}

// writePrompts は emit / collect が書き出したプロンプトのパスを stdout に 1 行ずつ、次の手順を stderr に出す。
// stdout はパスだけにする（スキルがそのまま 1 行 = 1 サブエージェントとして使えるように）。
func writePrompts(out, errOut io.Writer, prompts []string, next string) {
	for _, p := range prompts {
		_, _ = fmt.Fprintln(out, p)
	}
	_, _ = fmt.Fprintf(errOut, "\n%d 件のプロンプトを書き出した。プロンプト 1 つにつき新しい spec-gate-llm-worker サブエージェントを 1 体起動し、\n"+
		"「Read <プロンプトのパス> and follow it.」とだけ渡して並列に答えさせる（説明や要約を足さない・1 体に 2 つ渡さない）。\n"+
		"全員が書き終えたら %s\n", len(prompts), next)
}

const extractHelp = `extract は LLM を直接呼ばない。次のどちらかで実行する。

1. サブエージェントに委譲する（推奨。手順は .claude/skills/spec-gate-extract/SKILL.md）
     spec-gate extract <ref> --runs N --emit
       run ごとに自己完結したプロンプトを .spec-gate-llm/ に書き出し、そのパスを表示する
     プロンプト 1 つにつき新しい spec-gate-llm-worker サブエージェントを 1 体、並列に起動し、
       「Read <プロンプトのパス> and follow it.」とだけ渡す（各 run は互いの回答を見ない）
     spec-gate extract <ref> --collect
       N 個の回答を合議して claims/_pending/ に置く

2. LLM コマンドを直接呼ぶ
     SPEC_GATE_EXTRACT_CMD='claude -p' spec-gate extract <ref> --runs N`

func newExtractCmd(a **app) *cobra.Command {
	var runs int
	var srcType string
	var emit, collect bool
	cmd := &cobra.Command{
		Use:   "extract <ref>",
		Short: "仕様から claim 候補を N 回独立に抽出し、一致数つきで claims/_pending/ に置く（合議）",
		Long: `例:
  spec-gate extract docs/spec/rounding.md#端数処理 --runs 3 --emit
  （各プロンプトを spec-gate-llm-worker サブエージェントに 1 つずつ答えさせる）
  spec-gate extract docs/spec/rounding.md#端数処理 --collect

  SPEC_GATE_EXTRACT_CMD='claude -p' spec-gate extract --type mermaid docs/arch/layers.mmd --runs 5

spec-gate 自身は LLM を呼ばない。--emit / --collect でサブエージェントに委譲するか、
環境変数 SPEC_GATE_EXTRACT_CMD に LLM コマンド（stdin に prompt、stdout に応答）を指定する。
作業ディレクトリは .spec-gate-llm/（collect が終わったら消す）。`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := pickLLMMode(emit, collect, os.Getenv("SPEC_GATE_EXTRACT_CMD"), extractHelp)
			if err != nil {
				return err
			}
			ref := args[0]
			svc := (*a).extract
			var cands []services.Candidate
			switch mode {
			case modeEmit:
				prompts, err := svc.Emit(cmd.Context(), domain.SourceType(srcType), ref, runs)
				if err != nil {
					return err
				}
				writePrompts(cmd.OutOrStdout(), cmd.ErrOrStderr(), prompts, fmt.Sprintf("spec-gate extract %q --collect を実行する。", ref))
				return nil
			case modeCollect:
				cands, err = svc.Collect(cmd.Context(), ref)
			default:
				cands, err = svc.Extract(cmd.Context(), domain.SourceType(srcType), ref, runs)
			}
			if err != nil {
				return err
			}
			writeCandidates(cmd.OutOrStdout(), cands)
			return nil
		},
	}
	cmd.Flags().IntVar(&runs, "runs", 3, "独立抽出の回数")
	cmd.Flags().StringVar(&srcType, "type", "markdown", "source type (markdown|mermaid|figma|image)")
	cmd.Flags().BoolVar(&emit, "emit", false, "LLM を呼ばず、run ごとのプロンプトを .spec-gate-llm/ に書き出してパスを表示する")
	cmd.Flags().BoolVar(&collect, "collect", false, "サブエージェントの回答を読み、合議して claims/_pending/ に置く（--runs / --type は emit 時のもの）")
	return cmd
}

func writeCandidates(w io.Writer, cands []services.Candidate) {
	for _, c := range cands {
		flag := "  "
		if c.Agreements < (c.Runs+1)/2 {
			flag = "? "
		}
		_, _ = fmt.Fprintf(w, "%s%d/%d  %-12s %s\n      → %s\n", flag, c.Agreements, c.Runs, c.Claim.ID, c.Claim.Title, c.Path)
	}
	_, _ = fmt.Fprintf(w, "\n%d candidates. 「?」は過半数に満たない（要確認）。承認は内容を見て claims/<領域>/ へ移してから approve。\n", len(cands))
}
