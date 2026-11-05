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
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/evidence"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/extractor"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/llmwork"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/planfile"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/sourcefile"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/sourceregistry"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/tracelog"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/vcs"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/workspace"
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
	lint     *services.LintService
	mutation *services.MutationService
	plans    domain.PlanRepository
	claims   domain.ClaimRepository
	// holdout は check / lint / leak が読む（エージェントには見せない）
	holdout domain.HoldoutRepository
	// holdoutAdd は spec-gate holdout add（人間だけが使う）
	holdoutAdd *services.HoldoutService
	// trace は通り方のログ（.spec-gate/trace.jsonl）
	trace *services.TraceService
	// adapters は source type ごとの仕様の読み方。サービスを組み立てるコマンド（reverse など）が共有する
	adapters map[domain.SourceType]domain.SourceAdapter
	// work は LLM 処理をサブエージェントに委譲するときの作業ディレクトリ（.spec-gate-llm/）
	work domain.LLMWorkspace
	// probe は example claim の試し実行（examples だけを binding に流す）
	probe domain.BindingProbe
}

type utcClock struct{}

func (utcClock) Now() string { return time.Now().UTC().Format(time.RFC3339) }

func newApp(root string) *app {
	claims := claimrepo.New(root)
	holdout := claimrepo.NewHoldout(root)
	plans := planfile.New(root)
	runner := checker.ShellRunner{Dir: root, Timeout: 5 * time.Minute}
	mermaid := adapter.NewMermaid(root)

	adapters := map[domain.SourceType]domain.SourceAdapter{
		domain.SourceMarkdown: adapter.NewMarkdown(root),
		domain.SourceMermaid:  mermaid,
		domain.SourceFigma:    adapter.NewFigma(),
		domain.SourceImage:    adapter.NewImage(root),
	}
	example := checker.NewExample(runner)
	checkers := map[domain.ClaimKind]domain.Checker{
		domain.KindExample:   example,
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
	work := llmwork.New(root)
	extract := &services.ExtractService{
		Claims: claims, Adapters: adapters, Prompter: extractor.Prompter{}, Work: work,
		Probe: example, Pending: claims,
	}
	// LLM を直接呼ぶのは環境変数でコマンドを指定したときだけ（意図せず LLM を呼ばない）
	if cmd := os.Getenv("SPEC_GATE_EXTRACT_CMD"); cmd != "" {
		extract.Extractor = extractor.ClaudeCLI{Command: cmd}
	}

	mutation := &services.MutationService{Plans: plans, Claims: claims, Files: sourcefile.New(root), Runner: runner, Paths: workspace.Paths{Root: root}, Deps: workspace.NewGoDeps(root)}

	return &app{
		root:     root,
		claims:   claims,
		plans:    plans,
		mutation: mutation,
		holdout:  holdout,
		adapters: adapters,
		check: &services.CheckService{
			Claims: claims, Plans: plans, Adapters: adapters, Checkers: checkers,
			Runner: runner, Changes: vcs.Git{Dir: root}, Paths: workspace.Paths{Root: root}, Deps: workspace.NewGoDeps(root), Holdout: holdout, DiagramEdges: diagramEdges, Evidence: evidence.New(root),
		},
		lint:       &services.LintService{Claims: claims, Holdout: holdout},
		holdoutAdd: &services.HoldoutService{Claims: claims, Writer: holdout},
		approve:    &services.ApproveService{Claims: claims, Adapters: adapters, Clock: utcClock{}, Probe: example},
		coverage:   &services.CoverageService{Claims: claims, Ambiguous: claims, Sources: sourceregistry.New(root), Adapters: adapters},
		extract:    extract,
		probe:      example,
		work:       work,
		plan:       &services.PlanService{Plans: plans, Clock: utcClock{}, Mutation: mutation},
		status:     &services.StatusService{Claims: claims, Plans: plans},
		trace:      &services.TraceService{Log: tracelog.New(root), Lines: vcs.Git{Dir: root}, Clock: utcClock{}},
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
  metrics  → claim の kind 別件数・割合と unverified 率（CI 用）
  trace    → claim ごとに、最後に pass になったときの直前の連続 fail 回数と変更行数（通り方のログ）
  reverse  → claim だけから仕様文を再生成し、原文と食い違いがないかを突き合わせる（逆変換）
  review   → _pending の候補をローカルの Web ページで平易に並べ、人間が承認する（承認 UI）。
             review ask / review record は Claude Code の質問ダイアログと PostToolUse hook で承認する（リモート承認）
  leak     → 実装ファイルに examples・holdout の値がそのまま書かれていないかを調べる
  holdout  → holdout add で holdout を足す（期待値はハッシュだけを保存。人間だけが使う）`,
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
		newMCPCmd(&a),
		newCoverageCmd(&a),
		newMetricsCmd(&a),
		newTraceCmd(&a),
		newExtractCmd(&a),
		newReverseCmd(&a),
		newLeakCmd(&a),
		newGuardCmd(&a),
		newAutoApproveCmd(&a),
		newAuditCmd(&a),
		newRevokeCmd(&a),
		newHoldoutCmd(&a),
		newPlanCmd(&a),
		newVersionCmd(),
	)
	if err := cmd.Execute(); err != nil {
		printErr(err)
		os.Exit(1)
	}
}
