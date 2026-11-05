#!/usr/bin/env bash
# .mcp.json から起動する承認 UI の MCP サーバ（spec-gate mcp）。承認の経路なので、bin/spec-gate の固定版で動かす
# （docs/spec/auto-approval.md「固定版で動かす」）。開発中のビルドで動かすと、ソースを書き換えるだけで承認を通せてしまう。
# stdout は MCP の通信なので、取得のログは bin/spec-gate が stderr に出す。このファイルは guard が保護する。
set -euo pipefail
cd "$(dirname "$0")/.."
exec bin/spec-gate mcp "$@"
