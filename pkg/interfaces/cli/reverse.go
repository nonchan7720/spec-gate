package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/adapter"
	"github.com/nonchan7720/spec-gate/pkg/infrastructure/reverser"
	"github.com/nonchan7720/spec-gate/pkg/services"
	"github.com/spf13/cobra"
)

func newReverseCmd(a **app) *cobra.Command {
	var strict bool
	cmd := &cobra.Command{
		Use:   "reverse [<claim-id>...]",
		Short: "claim だけから仕様文を再生成し、元の仕様と食い違いがないかを LLM で突き合わせる（逆変換）",
		Long: `claim の内容だけ（元の仕様は見せない）から仕様文を書き起こし、元の仕様の該当箇所と比べる。
claim にあって仕様に無い主張、仕様にあって claim に無い規範、矛盾を列挙する。承認前のレビュー補助。
claim-id を省略すると全 claim（_pending は対象外）。figma / image の claim と、仕様が読めない claim は skipped。

既定では常に exit 0。--strict を付けると mismatch / error があれば exit 1。

例:
  spec-gate reverse
  spec-gate reverse ROUND-001 --strict

LLM コマンドは環境変数 SPEC_GATE_REVERSE_CMD で差し替え可（既定: claude -p）。
stdin に prompt を受け取り、stdout に応答を返すコマンドであればよい。`,
		RunE: func(cmd *cobra.Command, args []string) error {
			rep, err := newReverseService(*a).Reverse(cmd.Context(), args)
			if err != nil {
				return err
			}
			writeReverseReport(os.Stdout, rep)
			if rep.Failed(strict) {
				os.Exit(1)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&strict, "strict", false, "mismatch / error があれば exit 1")
	return cmd
}

// newReverseService は reverse の依存を組み立てる。root.go の newApp を変えずに済むようここで作る。
// テキストを持つ source（markdown / mermaid）の adapter だけを渡す。figma / image は service 側で skipped になる。
func newReverseService(a *app) *services.ReverseService {
	command := os.Getenv("SPEC_GATE_REVERSE_CMD")
	if command == "" {
		command = "claude -p"
	}
	return &services.ReverseService{
		Claims: a.claims,
		Adapters: map[domain.SourceType]domain.SourceAdapter{
			domain.SourceMarkdown: adapter.NewMarkdown(a.root),
			domain.SourceMermaid:  adapter.NewMermaid(a.root),
		},
		Reverser: reverser.Command{Command: command, Dir: a.root, Timeout: 5 * time.Minute},
	}
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
