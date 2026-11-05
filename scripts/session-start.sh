#!/usr/bin/env bash
# SessionStart hook（.claude/settings.json）。cloud セッション（CLAUDE_CODE_REMOTE=true）でだけ、
# このリポジトリの開発に要るものをバックグラウンドで用意する。cloud 環境のセットアップスクリプトは
# 他のリポジトリと共有なので使わない。ローカル（Mac など）では何もしない。
#   1. 承認 UI の MCP サーバと hook が使う bin/spec-gate.real（bin/spec-gate はそれを呼ぶ入口。scripts/mcp.sh）。MCP はセッション開始と同時に起動し、
#      30 秒で接続を諦めるので最初に作る。Go だけ先に入れてビルドし、他のツールや Neo4j の pull を待たせない
#   2. 残りの mise のツール（depgraph / gh / gh-stack など）
#   3. docker デーモンと depgraph 用の Neo4j（docker-compose.yml。イメージの pull に数分かかる）
# hook 自体はすぐ終わる。進み具合は $LOG に書く。
[[ ${CLAUDE_CODE_REMOTE:-} == true ]] || exit 0
cd "$(dirname "$0")/.." || exit 0
LOG=${TMPDIR:-/tmp}/spec-gate-session-start.log

(
  set -x
  if command -v mise >/dev/null; then
    mise install go
    # mcp.sh と同じロックを取る。mcp.sh が先に起動していればその間は待ち、ビルドは 1 回で済む
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
