#!/usr/bin/env bash
# SessionStart hook（.claude/settings.json）。cloud セッション（CLAUDE_CODE_REMOTE=true）でだけ、
# このリポジトリの開発に要るものをバックグラウンドで用意する。cloud 環境のセットアップスクリプトは
# 他のリポジトリと共有なので使わない。ローカル（Mac など）では何もしない。
#   - mise のツール（go / depgraph / gh / gh-stack など）
#   - docker デーモンと depgraph 用の Neo4j（docker-compose.yml）
#   - 承認 UI の MCP サーバが使う bin/spec-gate（scripts/mcp.sh）
# hook 自体はすぐ終わる。進み具合は $LOG に書く。
[[ ${CLAUDE_CODE_REMOTE:-} == true ]] || exit 0
cd "$(dirname "$0")/.." || exit 0
LOG=${TMPDIR:-/tmp}/spec-gate-session-start.log

(
  set -x
  command -v mise >/dev/null && mise install
  if command -v dockerd >/dev/null && ! docker info >/dev/null 2>&1; then
    dockerd >"${TMPDIR:-/tmp}/spec-gate-dockerd.log" 2>&1 &
    for _ in $(seq 1 60); do docker info >/dev/null 2>&1 && break; sleep 1; done
  fi
  docker info >/dev/null 2>&1 && docker compose up -d
  mkdir -p bin && mise exec -- go build -o bin/spec-gate .
  echo "session-start: done"
) >"$LOG" 2>&1 &
exit 0
