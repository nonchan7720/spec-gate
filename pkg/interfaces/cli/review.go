package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/interfaces/handler/review"
	"github.com/spf13/cobra"
)

func newReviewCmd(a **app) *cobra.Command {
	var by, addr string
	cmd := &cobra.Command{
		Use:   "review",
		Short: "承認待ちの claim（claims/_pending/）をブラウザで確認・承認するローカル画面を開く",
		Long: `claims/_pending/ の候補を、YAML を見せずに一覧する Web 画面を 127.0.0.1 で起動する。
選んで「承認」すると claims/<領域>/（example / manual → spec、structure → arch、ui → ui）へ移し、
approve と同じ手順（lint・仕様のハッシュ記録）で承認する。拒否されたら _pending に戻して理由を表示する。
「却下」は候補ファイルを削除する。
「曖昧」は claim の誤りではなく仕様が決まっていないことの記録。理由を入力させ、候補を claims/_ambiguous/ に
移して先頭に「# ambiguous: <理由>」を書く（check の対象外。coverage は参照先の単位を「要確認」として出す）。
Ctrl+C で終了。`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if by == "" {
				by = currentUser()
			}
			ln, err := listenReview(addr)
			if err != nil {
				return err
			}
			svc := newLoggedReviewService(*a, by)
			srv := &http.Server{Handler: review.New(svc, by), ReadHeaderTimeout: 10 * time.Second}

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			go func() {
				<-ctx.Done()
				shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = srv.Shutdown(shutdown)
			}()

			fmt.Printf("承認画面: %s  （承認者: %s、Ctrl+C で終了）\n", reviewURL(ln), by)
			if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&by, "by", "", "承認者（既定: OS ユーザー名）")
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:7790", "待ち受けアドレス（ループバックのみ）")
	// リモート承認（質問ダイアログ + PostToolUse hook）
	cmd.AddCommand(newReviewAskCmd(a), newReviewRecordCmd(a), newReviewProbeCmd(a))
	return cmd
}

// listenReview はループバックアドレスでだけ待ち受ける。承認は人間の操作なので、他の端末から叩けてはいけない。
func listenReview(addr string) (net.Listener, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("--addr が不正: %w", err)
	}
	if host == "localhost" {
		host = "127.0.0.1"
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return nil, fmt.Errorf("--addr %s: 承認画面はループバック（127.0.0.1 など）でしか待ち受けない", addr)
	}
	return net.Listen("tcp", net.JoinHostPort(host, port))
}

func reviewURL(ln net.Listener) string {
	return "http://" + ln.Addr().String() + "/"
}

func newReviewProbeCmd(a **app) *cobra.Command {
	return &cobra.Command{
		Use:   "probe",
		Short: "承認待ちの example 候補を試し実行し直し、`# dry-run:` を書き換える",
		Long: `claims/_pending/ の binding のある kind: example の候補に examples だけを流し、結果を候補ファイルの
` + "`# dry-run:`" + ` に書く（pass / fail: n 件中 k 件が期待と違う / error: 理由）。候補を直したあとに使う。
ヘッダは表示と並び順にだけ使い、承認するときには必ず試し実行し直す。`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := newReviewService(*a).Probe(cmd.Context())
			if err != nil {
				return err
			}
			writeProbe(cmd.OutOrStdout(), res)
			return nil
		},
	}
}

func writeProbe(w io.Writer, res map[string]string) {
	names := make([]string, 0, len(res))
	for n := range res {
		names = append(names, n)
	}
	sort.Strings(names)
	counts := map[domain.DryRunState]int{}
	for _, n := range names {
		_, _ = fmt.Fprintf(w, "  %-24s %s\n", n, res[n])
		counts[domain.DryRunStateOf(res[n])]++
	}
	_, _ = fmt.Fprintf(w, "\n%d 件を試し実行: pass %d / fail %d / error %d\n", len(names), counts[domain.DryRunPass], counts[domain.DryRunFail], counts[domain.DryRunError])
}
