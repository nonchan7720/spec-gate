package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newHoldoutCmd(a **app) *cobra.Command {
	holdout := &cobra.Command{Use: "holdout", Short: "holdout（エージェントに見せない入出力例）の操作。人間だけが使う"}
	var in, out string
	add := &cobra.Command{
		Use:   "add <claim-id> --in <json> [--out -]",
		Short: "holdout に 1 件足す（期待値は標準入力から読み、正規化 JSON の SHA-256 だけを保存する）",
		Long: `holdout/<claim-id>.yaml の examples の末尾に in と out_sha256 を足す（ファイルが無ければ作る）。
期待する出力（JSON）は標準入力から読む。端末ならエコーなしで読み、パイプなら全体を読む。

  spec-gate holdout add ID-001 --in '{"amount": 100}'             # 端末で期待値を入力（表示されない）
  spec-gate holdout add ID-001 --in '{"amount": 100}' < out.json

--out <json> も使えるが、値がシェル履歴に残るので非推奨（警告を出す）。--out - は省略と同じ。
out の平文はどこにも保存も表示もしない。holdout を書くのは人間だけで、エージェントからの実行は hook-gate で拒否する。`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHoldoutAdd(os.Stdout, os.Stderr, os.Stdin, *a, args[0], in, out, cmd.Flags().Changed("out"))
		},
	}
	add.Flags().StringVar(&in, "in", "", "binding に渡す入力（JSON）")
	add.Flags().StringVar(&out, "out", "", "期待する出力（JSON）。省略か - なら標準入力から読む。値を渡すとシェル履歴に残るので非推奨")
	_ = add.MarkFlagRequired("in")
	holdout.AddCommand(add)
	return holdout
}

// runHoldoutAdd は --in の JSON と期待値の JSON を読み、holdout に 1 件足して書いたファイルを w に出す。
// 期待値は --out が明示（outSet）され "-" でなければその値を使って errw に警告を出し、そうでなければ stdin から読む。
// out の値はエラーにも出力にも出さない。
func runHoldoutAdd(w, errw io.Writer, stdin io.Reader, a *app, claimID, inJSON, outFlag string, outSet bool) error {
	var in, out any
	if err := json.Unmarshal([]byte(inJSON), &in); err != nil {
		return fmt.Errorf("--in が JSON ではない: %w", err)
	}
	outJSON, err := readHoldoutOut(errw, stdin, outFlag, outSet)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(outJSON)) == 0 {
		return errors.New("期待する出力が空")
	}
	if err := json.Unmarshal(outJSON, &out); err != nil {
		return errors.New("期待する出力が JSON ではない（値は表示しない）")
	}
	path, err := a.holdoutAdd.Add(claimID, in, out)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "✓ %s に holdout を 1 件足した（期待値は out_sha256 だけを保存）\n", path)
	return nil
}

// readHoldoutOut は期待値の JSON を返す。標準入力が端末ならプロンプトを errw に出してエコーなしで読む。
func readHoldoutOut(errw io.Writer, stdin io.Reader, outFlag string, outSet bool) ([]byte, error) {
	if outSet && outFlag != "-" {
		_, _ = fmt.Fprintln(errw, "警告: --out の値はシェル履歴に残る。--out を省略して標準入力から渡すこと")
		return []byte(outFlag), nil
	}
	if f, ok := stdin.(*os.File); ok {
		if fd := int(f.Fd()); term.IsTerminal(fd) { //nolint:gosec // ファイルディスクリプタは int に収まる
			_, _ = fmt.Fprint(errw, "期待する出力（JSON、表示されません）: ")
			b, err := term.ReadPassword(fd)
			_, _ = fmt.Fprintln(errw)
			if err != nil {
				return nil, errors.New("期待する出力を端末から読めない")
			}
			return b, nil
		}
	}
	b, err := io.ReadAll(stdin)
	if err != nil {
		return nil, errors.New("期待する出力を標準入力から読めない")
	}
	return b, nil
}
