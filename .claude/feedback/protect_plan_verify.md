---
name: protect_plan_verify
description: 凍結済みプランの verify 行を書き換えない（緩めて通すのを防ぐ）
type: feedback
count: 1
enforce:
  - event: post_edit
    path: 'plans/*.md'
    exclude_path: 'plans/TEMPLATE.md'
    check: 'cd "$CLAUDE_PROJECT_DIR" && go run . plan check "${FILE#"$CLAUDE_PROJECT_DIR"/}"'
    severity: block
    message: '凍結済みプランの verify 行が変更された。verify を元に戻し、コード側を直すこと。verify の変更が必要ならユーザーに相談する（再凍結は人間が行う）。'
---

`spec-gate plan freeze` 済みのプランの `verify:` 行は変更しない。チェック状態（`[ ]` → `[x]`）や本文の更新は自由。
未凍結のプランは verify を書いてよい（すべてのチェック項目に `verify:` が必須）。

**Excuse:** 「verify のコマンドが厳しすぎて通らないので、通るコマンドに書き換えた」

**指摘:** verify は実装前に固定した oracle（TDD のテストに相当）。後から緩めると「検証したことにする」だけになる。通らないならコードを直す。verify 自体が誤りなら人間に相談して再凍結してもらう。

**Why:** HANDOFF.md の決定「プランの verify: 行は実装前に凍結。緩めると stale」を、check を待たず編集した瞬間に止めるため。

**How to apply:** plans/*.md を編集したとき。post_edit で `spec-gate plan check` が凍結ハッシュとの不一致を検出したらブロックされる。
