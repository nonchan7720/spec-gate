#!/usr/bin/env bash
# PreToolUse hook（.claude/settings.json と .claude/agents/spec-gate-approver.md）から呼ぶ spec-gate guard。
# guard が動かないときに素通りさせない（止める側に倒す）。Claude Code は exit 2 だけを「止める」と扱い、
# それ以外の失敗（バイナリが無い exit 127 など）は素通りさせるため。
#   1. bin/spec-gate があればそれを使う（cloud セッションでは SessionStart hook が最初にビルドする）
#   2. 無ければ go run で判定する（初回は遅いが正しく判定する）
#   3. どちらも動かなければ exit 2 で止め、理由を出す
# stdin（hook の入力）は 1 回しか読めないので、先に読んでおいて渡す。
root=${CLAUDE_PROJECT_DIR:-$(cd "$(dirname "$0")/.." && pwd)}
input=$(cat)
bin=$root/bin/spec-gate

run() {
  printf '%s' "$input" | "$@" -C "$root" guard
}

if [[ -x $bin ]]; then
  run "$bin"
  code=$?
elif command -v go >/dev/null 2>&1; then
  cd "$root" && run go run .
  code=$?
else
  code=127
fi

case $code in
  0 | 2) exit "$code" ;;
  *)
    echo "spec-gate guard: guard を実行できない（exit $code）ので止める。bin/spec-gate をビルドする（go build -o bin/spec-gate .）か、go を入れる" >&2
    exit 2
    ;;
esac
