---
title: 承認の自動化（HOTL 承認）の実装
status: draft        # 未決事項が決まり、claim を承認し、claims を埋めて凍結してから active にする
claims: []           # docs/spec/auto-approval.md から抽出・承認した claim の ID を入れる
mutation: 0.8        # auto-eligible / auto-breaker の binding が触る実装に対して
---

# 承認の自動化（HOTL 承認）の実装

## 目的

docs/spec/auto-approval.md を実装し、方針と、エージェントが作れない証拠を満たした claim 候補を機械が承認できるようにする。
人間は `audit` で見張り、`revoke` で差し戻し、ブレーカーと `policy.yaml` で止められる。

## 進め方（spec-gate で spec-gate を作る）

1. 人間: docs/spec/auto-approval.md の「未決事項」を決め、本文に書き込む
2. エージェント: `spec-gate-extract` スキルで節ごとに抽出する（`spec-gate extract "docs/spec/auto-approval.md#<見出し>" --runs 3 --emit` → サブエージェント → `--collect`）
3. 人間: MCP Apps の承認 UI（`mcp__spec-gate__review`）か `spec-gate review` で承認する。`claims/<領域>/` は `auto` を使う
4. エージェント: 承認された ID を上の `claims:` に入れる（プランの verify は変えない）。人間: `spec-gate plan freeze plans/2026-11-auto-approval.md`
5. 人間: `status: active` にする。以後、エージェントは `spec-gate check --changed` が通るまで実装する

## チェック項目

- [ ] 仕様の規範の節がすべて claim で覆われている
  verify: ! go run . coverage 2>/dev/null | sed -n '/^## docs\/spec\/auto-approval.md/,/^## /p' | grep -q '✗'
- [ ] 自動承認の判定とブレーカーの binding がある
  verify: go test ./tools/bind/ -run 'TestAutoEligible|TestAutoBreaker'
- [ ] 偽の LLM コマンドで、抽出 → 逆変換 → 試し実行 → 承認 → 証拠の記録が通る
  verify: go test ./pkg/services/ -run TestAutoApprove
- [ ] 環境変数の LLM コマンドが自動承認に使われない
  verify: go test ./pkg/services/ -run TestAutoApproveIgnoresEnvCommand
- [ ] 証拠の無い・書き換えられた自動承認は unapproved になる
  verify: go test ./pkg/services/ -run TestCheckAutoEvidence
- [ ] revoke はエージェントに拒否される
  verify: grep -q 'revoke' .claude/feedback/protect_spec_claims.md
- [ ] 承認済みの claim（このプランのものを含む）がすべて blocking でない
  verify: go run . check --no-plans
- [ ] 全体のテストと lint が通る
  verify: go vet ./... && go test ./...

## 備考

- 証拠を作る LLM 呼び出しは `.spec-gate/policy.yaml` の `llm_command` だけ。テストでは固定の回答を返すスクリプトを `llm_command` にする
- 既存の `review-decisions.jsonl` の形（リモート承認・MCP Apps）は変えず、行を足す経路（CLI の approve・承認画面・auto・revoke）を増やす
- まとめ承認（`approve --pending`）の対象の条件と、自動承認の判定の 3〜6 は同じ考え方。判定の関数は共有する（depgraph の duplicate-code を出さない）
