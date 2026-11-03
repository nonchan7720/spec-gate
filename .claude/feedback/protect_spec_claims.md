---
name: protect_spec_claims
description: 承認済み claim（claims/**）とハーネス状態（.spec-gate/）をエージェントが書き換えない。承認・凍結は人間が行う
type: feedback
count: 1
enforce:
  - event: pre_edit
    path: 'claims/**'
    exclude_path: 'claims/_pending/**'
    severity: deny
    message: 'claims/** は承認済みの仕様主張で編集禁止。コード側を仕様に合わせること。claim が間違っていると考えるなら claims/_pending/ に修正案を置き、根拠をユーザーに伝えて止まる。'
  - event: pre_edit
    path: '.spec-gate/**'
    severity: deny
    message: '.spec-gate/ はハーネスのロック・状態ファイルで編集禁止。'
  - event: pre_bash
    # リダイレクトは書き込み先そのもの、コマンドは同じ行の引数だけを見る（ヒアドキュメント本文や文字列中の言及は対象外）
    when: '(>{1,2}\s*["'']?|(\bsed\s+-i|\btee\b|\bmv\b|\bcp\b|\brm\b|\btruncate\b|\bgit\s+(checkout|restore)\b)[^|;&\n]*[\s"''=])(\./)?(claims/(?!_pending/)|\.spec-gate/)'
    severity: deny
    message: 'claims/**（_pending 以外）と .spec-gate/ はシェル経由でも書き換え禁止。'
  - event: pre_bash
    # コマンドの先頭（行頭・; & | ( の直後）で実行されたときだけ。mise exec / pnpm gate 経由も含む
    when: '(^|[\n;&|(]\s*)(mise\s+(exec|x)\s+--\s+)?((\S*/)?spec-gate|go\s+run\s+\S+|(pnpm|npm\s+run)\s+gate(\s+--)?)\s+(approve|plan\s+freeze)\b'
    severity: deny
    message: 'claim の承認（approve）とプランの凍結（plan freeze）は人間の操作。エージェントは実行せず、ユーザーに依頼すること。'
  - event: pre_bash
    # review record は PostToolUse hook（ハーネス）だけが起動する。直接実行すると偽の stdin で承認できてしまう。
    # コマンドの位置（行頭・; & | ( の直後・sh -c の引数の先頭）で実行されたときだけ。環境変数・env・mise exec・-C/--root を挟んでも拾う
    when: '(^|[\n;&|(]\s*|\b(ba|z)?sh\s+-c\s+["'']\s*)(\w+=\S*\s+)*(env\s+(\w+=\S*\s+)*)?(mise\s+(exec|x)\s+--\s+)?((\S*/)?spec-gate|go\s+run\s+\S+|(pnpm|npm\s+run)\s+gate(\s+--)?)((\s+(-C|--root)(\s+|=)\S+)*)\s+review((\s+(-C|--root)(\s+|=)\S+)*)\s+record\b'
    severity: deny
    message: 'spec-gate review record はリモート承認の PostToolUse hook 専用（ハーネスが AskUserQuestion の回答を渡す）。エージェントが直接実行してはいけない。承認は review ask の結果で AskUserQuestion を出し、ユーザーに選んでもらうこと。'
  - event: pre_bash
    # 承認画面（spec-gate review）の変更系 API をシェルから叩かない。curl などがコマンドの位置にあるときだけ（grep や git log の検索語は対象外）
    when: '(^|[\n;&|(]\s*)(\w+=\S*\s+)*((curl|wget|https?|xh|xhs)\s[^\n;&|]*|(python3?|node|deno|bun|ruby|perl)\s[^\n]*)/api/(approve|reject|ambiguous)\b'
    severity: deny
    message: '承認画面の /api/approve・/api/reject・/api/ambiguous は人間がブラウザで押すためのもの。エージェントがシェルから叩いてはいけない。ユーザーに依頼すること。'
---

承認済みの claim（`claims/**`、`_pending/` を除く）とハーネスの状態（`.spec-gate/`）はエージェントが書き換えない。
`spec-gate approve` と `spec-gate plan freeze` も人間が叩く。

**Excuse:** 「check が通らないので claim の期待値を実装に合わせて直した」「凍結し直せば stale が消える」

**指摘:** claim と凍結は「AI の自己申告を使わない」ための oracle。エージェントが書き換えた瞬間にハーネスは素通りになる。仕様とズレているなら、直すのはコード。claim 自体が誤りなら `claims/_pending/` に修正案を置いて人間に判断を仰ぐ。

**Why:** HANDOFF.md の設計方針「claim は AI が編集できない」「人間承認は変換時の1回だけ」を機械的に担保するため（hook-gate 側の保護が無いと「嘘をつけない構造」が片輪になる）。

**How to apply:** このリポジトリで claims/・.spec-gate/ を Edit/Write/Bash で変更しようとしたとき、または approve / plan freeze を実行しようとしたとき。
