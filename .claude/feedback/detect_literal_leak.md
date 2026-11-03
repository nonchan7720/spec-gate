---
name: detect_literal_leak
description: 実装ファイルに claim の examples・holdout の値をそのまま書かない（例の値で分岐して通す実装を警告する）
type: feedback
count: 1
enforce:
  - event: post_edit
    path: '**/*.{go,ts,tsx,js,jsx,mjs,cjs}'
    # テスト・仕様・claim・holdout は値が書かれていて当然なので対象外（spec-gate leak 側でも同じ除外をする）
    exclude_path:
      - '**/*_test.go'
      - '**/*.test.ts'
      - '**/*.spec.ts'
      - 'claims/**'
      - 'holdout/**'
      - 'docs/**'
    check: 'cd "$CLAUDE_PROJECT_DIR" && go run . leak "$FILE"'
    severity: warn
    message: '実装ファイルに claim の examples・holdout の値がそのまま書かれている（spec-gate leak）。例の値で分岐・返却せず、仕様どおりの一般的な処理にすること。定数として正当な値なら、その理由をユーザーに伝える。'
---

実装ファイルに claim の examples・holdout の値（数字 3 桁以上の数値・4 文字以上の文字列）をそのまま書かない。
`spec-gate leak <file>` が値と行番号を出す（holdout の値は表示しない）。

**Excuse:** 「examples の 1234.56 → 1234 だけ通ればよいので `if x == 1234.56 { return 1234 }` と書いた」

**指摘:** examples は仕様の代表例で、実装の分岐表ではない。例の値を直書きすると claim は通っても仕様は満たさない。
holdout（見せない入出力例）とランダム検証で落ちるうえ、人間のレビューでも見抜きにくい。

**Why:** docs/spec/harness.md「リテラル漏れ検知」。エージェントが「例の数件だけ通る実装」で claim を満たすのを、編集した時点で人間とエージェントに見えるようにする。

**How to apply:** このリポジトリで実装ファイル（テスト・docs・claims・holdout 以外）を編集したとき。post_edit で `spec-gate leak` が値を見つけたら警告される。
仕様の文言そのもの（画面の見出しなど）が値として claim に入っている場合は、正当な一致なのでそのままでよい。
