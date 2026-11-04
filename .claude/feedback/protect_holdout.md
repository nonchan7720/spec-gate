---
name: protect_holdout
description: holdout（holdout/**、エージェントに見せない入出力例）をエージェントが読まない・書かない。読むのは spec-gate 本体（check / lint）だけ
type: feedback
count: 1
enforce:
  - event: pre_edit
    path: 'holdout/**'
    severity: deny
    message: 'holdout/** はエージェントに見せない入出力例で、編集も読み取りも禁止。holdout の追加・変更は人間が行う。'
  - event: pre_bash
    # holdout/ をパスとして含むコマンドは、読み取り（cat / grep / git show HEAD:holdout/... など）も書き込み（> / tee / cp / mv / rm など）もまとめて拒否する。
    # 直前が英数字・_・- の語（myholdout/ など）は別のパスなので対象外
    when: '(^|[^\w-])holdout/'
    severity: deny
    message: 'holdout/ はシェル経由でも読み書き禁止（cat / less / head / tail / grep / rg / sed / awk / git show / git log -p / git diff など）。holdout は spec-gate check / lint だけが読む。結果は件数でしか出ないので、値を推測して実装に書かないこと。'
  - event: pre_bash
    # 末尾の / が無いディレクトリ指定（grep -r x holdout、ls holdout、git diff -- holdout など）。
    # 読み書きするコマンドと同じ行の引数だけを見る（コミットメッセージなどの単語としての holdout は対象外）
    when: '\b(cat|less|more|head|tail|grep|egrep|fgrep|rg|ag|ack|sed|awk|ls|find|tree|bat|nl|xxd|od|hexdump|strings|base64|cp|mv|rm|tar|zip|diff|tee|touch|mkdir|chmod|ln|python3?|node|ruby|perl|git\s+(show|log|diff|grep|blame|cat-file|checkout|restore|archive|add|rm|mv))\b[^|;&\n]*[\s"''=:](\./)?holdout(?=$|[\s"'';&|)])'
    severity: deny
    message: 'holdout/ はシェル経由でも読み書き禁止。holdout は spec-gate check / lint だけが読む。'
---

holdout（`holdout/<claim-id>.yaml`）は、エージェントに見せない入出力例。`spec-gate check` が examples に続けて binding に流し、
結果は件数（「holdout 2 件中 1 件が不一致（値は表示しない）」）でしか出さない。エージェントは holdout を読まない・書かない。

**Excuse:** 「holdout が不一致なので、どの入力で落ちているか holdout/ を見て確かめた」「git show で過去の holdout を見ただけ」

**指摘:** holdout は examples の数件だけを満たす実装（例の値の直書き）を落とすための oracle。中身を見た瞬間に、それはただの examples になる。
不一致なら、仕様（docs/spec）と公開の examples・properties から一般的な実装を考え直す。値を推測して実装に書かない。

**Why:** docs/spec/harness.md「holdout の読み取り拒否」。`.claude/settings.json` の `permissions.deny`（Read / Edit / Write）と合わせて、
Claude Code のツールとシェルの両方から holdout を塞ぐ。

**How to apply:** このリポジトリで holdout/ を Read / Edit / Write / Bash で読み書きしようとしたとき。
再帰検索（`grep -r` / `rg` をリポジトリ全体に）や、パスを付けない `git log -p` / `git diff` / `git show <commit>` も holdout を表示しうるので、
holdout/ を除外して使う（例: `rg pattern -g '!holdout'`）。
