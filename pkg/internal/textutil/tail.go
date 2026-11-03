// Package textutil はレイヤーをまたいで使う文字列処理。
package textutil

import (
	"fmt"
	"strings"
)

// Tail は末尾 n 行を返す。コマンド出力のエラーは末尾に出ることが多いので、要点だけ見せるのに使う。
func Tail(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return fmt.Sprintf("… (%d lines omitted)\n%s", len(lines)-n, strings.Join(lines[len(lines)-n:], "\n"))
}
