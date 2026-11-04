---
name: stack_prs_on_the_top
description: このリポジトリの PR は GitHub の stacked PR（stack として登録する機能）で積む。base ブランチを一列につなぐだけでは stack にならない
type: feedback
count: 3
---

このリポジトリの PR は、GitHub の stacked pull requests（https://docs.github.com/en/pull-requests/get-started/about-stacked-prs）で積む。
stack は GitHub 上に登録される単位で、PR を stack に入れるのは `gh stack`（`gh extension install github/gh-stack`）か PR 画面の stack の操作。
base ブランチを前の PR のブランチに向けただけでは stack に入らない（一列に並んでいても別物）。

- 新しい作業は、開いている stack の一番上で `gh stack add` → `gh stack submit`（既存の PR は `gh stack link <stack 番号> <PR>`）で stack に足す
- 下の PR が変わったら `gh stack rebase --upstack` / `gh stack sync` でそろえる。衝突は `gh stack rebase --continue`
- マージは `gh stack merge <PR>`（または PR 画面のマージボックス）で、下から連続した範囲をまとめて。一番上をマージすれば全部入る。
  通常のマージ API（`PUT /pulls/{n}/merge`）は stack を扱えない（403）。stack の途中の PR を 1 本ずつ squash マージしない
- 自分の環境で `gh stack` が使えない（GitHub の操作が MCP ツールだけなど）なら、PR を作る前にそう伝え、stack への登録を利用者に頼む

**Excuse:** 「#28 の上に base を向けて PR を作った」「base を #33 に付け替えて一列にしたので、一番上をマージすれば入る」「stack の下から 1 本ずつ squash マージすればよい」

**指摘:** stack は base のつなぎ方ではなく GitHub に登録された単位。登録していない PR は stack のマージに含まれず、途中の PR を通常の API で 1 本ずつマージすると stack の外で squash されて衝突が出る。何度も伝えている。

**Why:** 利用者から「stack 使ってって何回も言ってる」「stack がわかってないと思う。GitHub にある stack 機能やで」「1 本になればいいってことじゃないで」と言われた。

**How to apply:** PR を作る・マージするときは毎回。作る前に stack の一番上と、自分が `gh stack` を使えるかを確かめる。
