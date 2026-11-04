#!/usr/bin/env bash
# .mcp.json から起動する承認 UI の MCP サーバ（spec-gate mcp）。
# 毎回 `go run` するとビルドで MCP の起動タイムアウト（既定 30 秒）を超えるので、bin/spec-gate を使い回し、
# Go のソースか画面の HTML が bin より新しいときだけビルドし直す。ビルドのログは stderr に出す（stdout は MCP の通信）。
set -euo pipefail
cd "$(dirname "$0")/.."

bin=bin/spec-gate
if [[ ! -x $bin ]] || [[ -n $(find . -path ./node_modules -prune -o -path ./.git -prune -o \
  \( -name '*.go' -o -name 'go.mod' -o -name 'go.sum' -o -path './pkg/interfaces/mcpapp/assets/*' \) \
  -newer "$bin" -print -quit) ]]; then
  echo "spec-gate mcp: building $bin" >&2
  mkdir -p bin
  if command -v mise >/dev/null; then
    mise exec -- go build -o "$bin" . >&2
  else
    go build -o "$bin" . >&2
  fi
fi
exec "$bin" mcp "$@"
