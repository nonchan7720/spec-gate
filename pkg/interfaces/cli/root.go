// Package cli は cobra コマンドと依存の組み立て（composition root）。
package cli

import (
	"os"
	"path/filepath"
	"time"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/adapter"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/checker"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/claimrepo"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/extractor"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/planfile"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/sourceregistry"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/vcs"
	"github.com/nonchan7720/spec-gate/pkg/services"
	"github.com/nonchan7720/spec-gate/pkg/version"
	"github.com/spf13/cobra"
)

type app struct {
	root     string
	check    *services.CheckService
	approve  *services.ApproveService
	coverage *services.CoverageService
	extract  *services.ExtractService
	plan     *services.PlanService
	status   *services.StatusService
	claims   domain.ClaimRepository
	// adapters は source type ごとの仕様の読み方。サービスを組み立てるコマンド（reverse など）が共有する
	adapters map[domain.SourceType]domain.SourceAdapter
}

type utcClock struct{}

func (utcClock) Now() string { return time.Now().UTC().Format(time.RFC3339) }

func newApp(root string) *app {
	claims := claimrepo.New(root)
	plans := planfile.New(root)
	runner := checker.ShellRunner{Dir: root, Timeout: 5 * time.Minute}
	mermaid := adapter.NewMermaid(root)

	adapters := map[domain.SourceType]domain.SourceAdapter{
		domain.SourceMarkdown: adapter.NewMarkdown(root),
		domain.SourceMermaid:  mermaid,
		domain.SourceFigma:    adapter.NewFigma(),
		domain.SourceImage:    adapter.NewImage(root),
	}
	checkers := map[domain.ClaimKind]domain.Checker{
		domain.KindExample:   checker.NewExample(runner),
		domain.KindStructure: checker.NewDepgraph(root, runner, os.Getenv("SPEC_GATE_DEPGRAPH_CMD")),
		domain.KindUI:        checker.NewUI(root, runner),
	}
	diagramEdges := func(ref string) (map[string]bool, error) {
		edges, err := mermaid.Edges(ref)
		if err != nil {
			return nil, err
		}
		set := map[string]bool{}
		for _, e := range edges {
			set[e] = true
		}
		return set, nil
	}
	extractCmd := os.Getenv("SPEC_GATE_EXTRACT_CMD")
	if extractCmd == "" {
		extractCmd = "claude -p"
	}

	return &app{
		root:     root,
		claims:   claims,
		adapters: adapters,
		check: &services.CheckService{
			Claims: claims, Plans: plans, Adapters: adapters, Checkers: checkers,
			Runner: runner, Changes: vcs.Git{Dir: root}, DiagramEdges: diagramEdges,
		},
		approve:  &services.ApproveService{Claims: claims, Adapters: adapters, Clock: utcClock{}},
		coverage: &services.CoverageService{Claims: claims, Sources: sourceregistry.New(root), Adapters: adapters},
		extract: &services.ExtractService{
			Claims: claims, Adapters: adapters,
			Extractor: extractor.ClaudeCLI{Command: extractCmd, PromptFile: filepath.Join(root, "pkg/infrastructure/extractor/prompt.md")},
		},
		plan:   &services.PlanService{Plans: plans, Clock: utcClock{}},
		status: &services.StatusService{Claims: claims, Plans: plans},
	}
}

// Execute はエントリポイント。
func Execute() {
	var root string
	var a *app
	cmd := &cobra.Command{
		Use:   "spec-gate",
		Short: "仕様（Markdown / Mermaid / Figma）とプランが AI コードで守られているかを機械的に判定する",
		Long: `spec-gate は仕様を「claim（検証可能な主張）」に変換し、exit code で守られているかを判定する。
エージェントの自己申告は使わない。

  extract  → 仕様から claim 候補を合議抽出（claims/_pending/）
  lint     → claim の内部整合（examples vs properties、claim 間の矛盾）
  approve  → 人間が承認し、仕様のハッシュを凍結
  check    → 承認 → ドリフト → 種類別チェック → プランの verify を実行
  coverage → 仕様のうち claim に紐づかない箇所を列挙
  reverse  → claim だけから仕様文を再生成し、原文と食い違いがないかを突き合わせる（逆変換）`,
		Version:       version.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if root == "" {
				root, _ = os.Getwd()
			}
			abs, err := filepath.Abs(root)
			if err != nil {
				return err
			}
			a = newApp(abs)
			return nil
		},
	}
	cmd.SetVersionTemplate(versionTemplate())
	cmd.PersistentFlags().StringVarP(&root, "root", "C", "", "プロジェクトルート（既定: カレント）")
	cmd.AddCommand(
		newCheckCmd(&a),
		newStatusCmd(&a),
		newLintCmd(&a),
		newApproveCmd(&a),
		newReviewCmd(&a),
		newCoverageCmd(&a),
		newExtractCmd(&a),
		newReverseCmd(&a),
		newPlanCmd(&a),
		newVersionCmd(),
	)
	if err := cmd.Execute(); err != nil {
		printErr(err)
		os.Exit(1)
	}
}
