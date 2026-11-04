package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/services"
	"github.com/spf13/cobra"
)

func newLeakCmd(a **app) *cobra.Command {
	return &cobra.Command{
		Use:   "leak <file>...",
		Short: "実装ファイルに claim の examples・holdout の値がそのまま書かれていないかを調べる（見つかれば exit 1）",
		Long: `数値は数字 3 桁以上、文字列は 4 文字以上の値だけを対象にする。
テストファイル（_test.go / .test.ts / .spec.ts）と claims/・holdout/・docs/ は対象外。
holdout は期待値をハッシュでしか持たないので in の値だけを調べ、値は表示せず行番号だけを出す。`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			code, err := runLeak(os.Stdout, *a, args)
			if err != nil {
				return err
			}
			if code != 0 {
				os.Exit(code)
			}
			return nil
		},
	}
}

// runLeak はファイルを読んで漏れを調べ、レポートを w に書いて exit code を返す。
func runLeak(w io.Writer, a *app, args []string) (int, error) {
	files := make([]services.LeakFile, 0, len(args))
	for _, arg := range args {
		b, err := os.ReadFile(arg)
		if err != nil {
			return 0, err
		}
		files = append(files, services.LeakFile{Path: relToRoot(a.root, arg), Content: string(b)})
	}
	svc := &services.LeakService{Claims: a.claims, Holdout: a.holdout}
	rep, err := svc.Scan(files)
	if err != nil {
		return 0, err
	}
	if len(rep.Findings) == 0 {
		return 0, nil
	}
	for _, f := range rep.Findings {
		value := "holdout の値（値は表示しない）"
		if !f.Hidden {
			b, _ := json.Marshal(f.Value)
			value = string(b)
		}
		_, _ = fmt.Fprintf(w, "✗ %s:%d  %s\n", f.File, f.Line, value)
	}
	_, _ = fmt.Fprintf(w, "\n%d 件: claim の examples・holdout の値が実装にそのまま書かれている。例の値で分岐せず、仕様どおりの一般的な処理にすること。\n", len(rep.Findings))
	return 1, nil
}

// relToRoot はファイルの位置をプロジェクトルート相対にする（対象外の判定はルート相対で行う）。
// ルートの外ならそのまま返す。
func relToRoot(root, p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(rel)
}
