#!/usr/bin/env bash
# SessionStart hook（.claude/settings.json）。cloud セッション（CLAUDE_CODE_REMOTE=true）でだけ、
# このリポジトリの開発に要るものをバックグラウンドで用意する。cloud 環境のセットアップスクリプトは
# 他のリポジトリと共有なので使わない。ローカル（Mac など）では何もしない。
#   1. hook と承認 UI の MCP サーバ（scripts/mcp.sh）が使う固定版の spec-gate を取得する（bin/spec-gate --fetch-pinned）。
#      hook は 10 秒で時間切れになり、MCP は起動と同時に接続を試すので最初に取る
#   2. 開発用の bin/spec-gate.real。Go だけ先に入れてビルドし、他のツールや Neo4j の pull を待たせない
#   3. 残りの mise のツール（depgraph / gh / gh-stack など）
#   4. docker デーモンと depgraph 用の Neo4j（docker-compose.yml。イメージの pull に数分かかる）
# hook 自体はすぐ終わる。進み具合は $LOG に書く。
[[ ${CLAUDE_CODE_REMOTE:-} == true ]] || exit 0
cd "$(dirname "$0")/.." || exit 0
LOG=${TMPDIR:-/tmp}/spec-gate-session-start.log

(
  set -x
  bin/spec-gate --fetch-pinned
  if command -v mise >/dev/null; then
    mise install go
    # bin/spec-gate と同じロックを取る。先にビルドが始まっていればその間は待ち、ビルドは 1 回で済む
    mkdir -p bin && flock bin/.build.lock mise exec -- go build -o bin/spec-gate.real .
    mise install
  fi
  if command -v dockerd >/dev/null && ! docker info >/dev/null 2>&1; then
    dockerd >"${TMPDIR:-/tmp}/spec-gate-dockerd.log" 2>&1 &
    for _ in $(seq 1 60); do docker info >/dev/null 2>&1 && break; sleep 1; done
  fi
  docker info >/dev/null 2>&1 && docker compose up -d
  echo "session-start: done"
) >"$LOG" 2>&1 &
exit 0
