---
name: protect_holdout_add
description: エージェントは spec-gate holdout add を実行しない。holdout（エージェントに見せない入出力例）を足すのは人間だけ
type: feedback
count: 1
enforce:
  - event: pre_bash
    # コマンドの位置（行頭、; & | ( ` $( の直後。環境変数の代入や mise exec ... -- を前に置いてもよい）で
    # spec-gate holdout add / go run <pkg> holdout add を呼ぶものだけを拒否する。
    # コミットメッセージや grep の引数の中の「holdout add」という文字列は対象外
    when: '(^|[;&|(`\n]|\$\()\s*(\w+=\S*\s+)*((mise|rtx)\s+(exec|x)\b[^;&|\n]*?--\s+)?(\S*/)?(spec-gate|go\s+run\s+\S+)\s+holdout\s+add\b'
    severity: deny
    message: 'spec-gate holdout add は人間だけが使うコマンド。エージェントは holdout を足さない・変えない。holdout を増やしたいときは、足したい入力と理由をユーザーに伝えて人間に実行してもらう。'
---

`spec-gate holdout add <claim-id> --in <json> --out <json>` は、holdout（`holdout/<claim-id>.yaml`）に入力と期待値のハッシュ（`out_sha256`）を足すコマンド。
holdout はエージェントに見せない oracle なので、足すのも人間だけ。

**Excuse:** 「holdout が足りないので、仕様の例から 1 件足しておいた」「テストのために holdout add を実行した」

**指摘:** エージェントが自分で holdout を足すと、自分の実装が通る入出力を oracle に書き込めてしまい、holdout の意味がなくなる。
期待値をハッシュにしても、書き込む側がエージェントなら防げない。

**Why:** docs/spec/harness.md「holdout examples」「holdout の読み取り拒否」。holdout を書くのは人間だけ。
`protect_holdout` は holdout/ のパスを含むコマンドを塞ぐが、`holdout add` はパスを含まずに holdout/ に書けるので、別に塞ぐ。

**How to apply:** このリポジトリで `spec-gate holdout add`、`go run . holdout add`、`mise exec -- go run . holdout add` などを Bash で実行しようとしたとき。
holdout add のテストは Go のテスト（一時ディレクトリ）で書き、リポジトリに対して実行しない。
