#!/usr/bin/env bash
# .mcp.json から起動する承認 UI の MCP サーバ（spec-gate mcp）。
# 毎回 `go run` するとビルドで MCP の起動タイムアウト（既定 30 秒）を超えるので、bin/spec-gate を使い回し、
# Go のソースか画面の HTML が bin より新しいときだけビルドし直す。ビルドのログは stderr に出す（stdout は MCP の通信）。
# cloud セッションでは SessionStart hook（scripts/session-start.sh）も同時に bin をビルドするので、
# 同じロック（bin/.build.lock）を取り、相手のビルド中ならそれを待って二重にビルドしない。
set -euo pipefail
cd "$(dirname "$0")/.."

bin=bin/spec-gate.real
stale() {
  [[ ! -x $bin ]] || [[ -n $(find . -path ./node_modules -prune -o -path ./.git -prune -o \
    \( -name '*.go' -o -name 'go.mod' -o -name 'go.sum' -o -path './pkg/interfaces/mcpapp/assets/*' \) \
    -newer "$bin" -print -quit) ]]
}

if stale; then
  mkdir -p bin
  exec 9>bin/.build.lock
  if command -v flock >/dev/null; then
    flock 9
  fi
  # ロックを待つ間に session-start.sh がビルドし終えていれば、もう一度ビルドしない
  if stale; then
    echo "spec-gate mcp: building $bin" >&2
    if command -v mise >/dev/null; then
      mise exec -- go build -o "$bin" . >&2
    else
      go build -o "$bin" . >&2
    fi
  fi
  # ロックを持ったままサーバを起動すると、session-start.sh のビルドがセッションの間ずっと待たされる
  exec 9>&-
fi
exec "$bin" mcp "$@"
