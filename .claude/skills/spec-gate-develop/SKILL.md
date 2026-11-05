---
name: spec-gate-develop
description: spec-gate 自身の機能を spec-gate で開発する手順（仕様 → 抽出 → 人間の承認 → プランの凍結 → 実装 → check）。「次の機能を進めて」「仕様から実装して」「プランを進めて」「自動承認（HOTL 承認）を実装して」と頼まれたときに使う。
---

# spec-gate で spec-gate を開発する

仕様（`docs/spec/*.md`）を正とし、claim とプランの verify で「できた」を機械に判定させる。
あなたが「実装した」と言っても完了にはならない。完了は `spec-gate check` と verify の exit code で決まる。

## 今どこにいるかを見る

```sh
go run . status                      # claim とプランの状態
go run . coverage                    # 仕様のうち claim の無い節（✗）と要確認（?）
```

進行中の仕様とプラン:

| 仕様 | プラン |
|---|---|
| `docs/spec/auto-approval.md`（承認の自動化。方針決定済み） | `plans/2026-11-auto-approval.md` |

## 手順

各段の「誰が」を守る。人間の段に来たら、何をしてほしいかを具体的に伝えて止まる。

1. **仕様を決める（人間）**。仕様に「未決事項」や【要確認】が残っていれば、抽出しない。
   論点と選択肢を整理して人間に聞く（AskUserQuestion）。答えを仕様に書き込むのはよいが、決めるのは人間
2. **抽出する（エージェント）**。節ごとに `spec-gate-extract` スキルで行う。ref は `docs/spec/<file>.md#<見出し>`。
   non_normative の節（`sources.yaml` の `non_normative`。背景・原則・決定の記録・変更履歴など）は抽出しない
3. **承認してもらう（人間）**。`mcp__spec-gate__review` で承認 UI を開き、承認を頼む（MCP が無ければ `spec-gate review` か `review ask`）。
   あなたは `approve` を実行しない（hook-gate が拒否する）。候補が誤っていると思えば、`claims/_pending/` の候補を直して理由を伝える
4. **プランに claim をつなぐ（エージェント → 人間）**。承認された ID をプランの `claims:` に入れる。verify 行は書き換えない。
   人間に `spec-gate plan freeze <plan>` と `status: active` を頼む
5. **実装する（エージェント）**。仕様の binding（`go run ./tools/bind <name>`）は `tools/bind` に足す。
   `go run . check --changed` と、プランの verify が全部通るまで終わらない。fail を消すために claim・verify・仕様を変えない
6. **PR にする（エージェント）**。`go vet ./...`・golangci-lint・`go test ./...`・`depgraph scan --root . --claude-md ''`（findings 0）を通してから

## 仕様の書き方（新しい機能を足すとき）

- 規範の節には、決定的に確かめられる部分の入出力例の表と binding（`go run ./tools/bind <name>`）を付ける。例の無い規則は manual にしかならない。
  表が無いのに入出力の規則がある節は、run ごとに例の形を発明して合議が割れる（承認方針の節で 32 候補中 25 件が 1/3 になった）
- 節は自己完結させる。抽出の担当者は自分の節の抜粋しか見ない。別の節の binding や表を参照すると `binding: guard` のような壊れた候補になる。
  同じ binding を 2 つの節で使うなら、それぞれの節に binding の行と、その節に関わる例の表を置く
- 新しい binding は、抽出の前に `tools/bind` に名前だけ登録する（null を返す stub）。未登録だと試し実行が error になり承認できない。
  stub なら fail になり、実装の前に承認できる

## 抽出のあと（エージェント）

- 節をまたいで claim ID がぶつかる（`AUTO-004` と `AUTO-004-3` が同じ `id`）。ID が重複した候補は承認できないので、節ごとの接頭辞で振り直す
  （中身は変えず、ファイル名・`id:`・案内コメントだけ）。そのあと `spec-gate review probe` で試し実行を更新する
- `review ask --limit 1 2>&1 >/dev/null` で、読めない候補と ID が重複する候補が無いことを確かめてから人間に承認を頼む
- 決まっていないことは本文に書かず「未決事項」に集める（non_normative にして sources.yaml に登録する）
- 新しい仕様ファイルは `sources.yaml` に登録する（coverage の対象になる）
