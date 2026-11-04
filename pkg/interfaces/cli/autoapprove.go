package cli

import (
	"fmt"
	"io"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/claimrepo"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/evidence"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/extractor"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/llmcmd"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/policyfile"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/reverser"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/tracelog"
	"github.com/nonchan7720/spec-gate/pkg/services"
	"github.com/spf13/cobra"
)

func newAutoApproveCmd(a **app) *cobra.Command {
	var area, srcType string
	cmd := &cobra.Command{
		Use:   "auto-approve <ref> --area <領域>",
		Short: "仕様 1 か所から候補を作り、方針と証拠が認めたものだけを承認する（承認役のサブエージェントだけが実行する）",
		Long: `docs/spec/auto-approval.md「自動承認」。LLM は .spec-gate/policy.yaml の llm_commands だけを使い（環境変数は使わない）、
抽出 → 合議 → 逆変換 → 試し実行 → 判定 を 1 つのコマンドの中で行う。判定が ok の候補は claims/<領域>/ に承認済み（by: auto）で置き、
それ以外は claims/_pending/ に「# auto: <判定>」つきで置く。判定のたびに証拠を .spec-gate/evidence/<ID>.json に書く。
エージェントは実行できない（spec-gate guard が、承認役 spec-gate-approver 以外からの実行を拒否する）。`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := newAutoApproveService(*a)
			rep, err := s.Run(cmd.Context(), domain.SourceType(srcType), args[0], area)
			writeAutoReport(cmd.OutOrStdout(), rep)
			return err
		},
	}
	cmd.Flags().StringVar(&area, "area", "spec", "承認した claim を置く claims/<領域>/")
	cmd.Flags().StringVar(&srcType, "type", "markdown", "source type (markdown|mermaid)")
	return cmd
}

func newAutoApproveService(a *app) *services.AutoApproveService {
	log := tracelog.NewDecisions(a.root)
	return &services.AutoApproveService{
		Policy: policyfile.New(a.root), Adapters: a.adapters, LLM: llmcmd.Shell{},
		Extract: extractor.Prompter{}, Reverse: reverser.Prompter{}, Probe: a.probe,
		Claims: a.claims, Placer: claimrepo.New(a.root), Evidence: evidence.New(a.root),
		Decisions: log, Log: log, Clock: utcClock{},
	}
}

func writeAutoReport(w io.Writer, rep services.AutoReport) {
	if rep.Paused {
		_, _ = fmt.Fprintln(w, "ブレーカーが止めている（直近の自動承認の差し戻しが多い）。承認はせず、候補と証拠だけを残す")
	}
	approved := 0
	for _, o := range rep.Outcomes {
		mark := "–"
		if o.Verdict == domain.AutoOK {
			mark, approved = "✓", approved+1
		}
		_, _ = fmt.Fprintf(w, "%s %-8s %-12s %s\n    合意 %s  試し実行 %s  逆変換 %s  → %s\n", mark, o.Verdict, o.ID, o.Title, o.Confidence, orDash(o.DryRun), o.Reverse, o.Path)
	}
	if len(rep.Outcomes) > 0 {
		_, _ = fmt.Fprintf(w, "\n%d 件中 %d 件を自動承認した。人間は spec-gate audit で見張り、spec-gate revoke で差し戻せる\n", len(rep.Outcomes), approved)
	}
}
