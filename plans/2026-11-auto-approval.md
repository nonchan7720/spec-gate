---
title: 承認の自動化（HOTL 承認）の実装
status: draft        # claim を承認し、claims を埋めて凍結してから active にする
claims: [POLICY-001, AUTO-001, ELIG-001, EVID-001, BREAKER-001, PROTECT-001, APPROVER-013, AUDIT-003, AUDIT-006, AUDIT-007, AUDIT-008]  # 承認済み（2026-11-04、リモート承認）。残りの候補は承認され次第足す
mutation: 0.8        # auto-eligible / auto-breaker の binding が触る実装に対して
---

# 承認の自動化（HOTL 承認）の実装

## 目的

docs/spec/auto-approval.md を実装し、方針と、エージェントが作れない証拠を満たした claim 候補を機械が承認できるようにする。
人間は `audit` で見張り、`revoke` で差し戻し、ブレーカーと `policy.yaml` で止められる。

## 進め方（spec-gate で spec-gate を作る）

1. ~~人間: 仕様の未決事項を決める~~（2026-11-04 決定。仕様の「決定の記録」）
2. エージェント: `spec-gate-extract` スキルで節ごとに抽出する（`spec-gate extract "docs/spec/auto-approval.md#<見出し>" --runs 3 --emit` → サブエージェント → `--collect`）
3. 人間: 承認する（スマホでは質問ダイアログのリモート承認。`spec-gate review ask --ref docs/spec/auto-approval.md` で絞れる）。承認画面の決まりで example / manual は `claims/spec/` に入る
4. エージェント: 承認された ID を上の `claims:` に入れる（プランの verify は変えない）。人間: `spec-gate plan freeze plans/2026-11-auto-approval.md`
5. 人間: `status: active` にする。以後、エージェントは `spec-gate check --changed` が通るまで実装する

## チェック項目

- [ ] 仕様の規範の節がすべて claim で覆われている
  verify: ! go run . coverage 2>/dev/null | sed -n '/^## docs\/spec\/auto-approval.md/,/^## /p' | grep -q '✗'
- [ ] 方針・run ごとのコマンド・判定・証拠の照合・ブレーカー・保護の判定の binding がある
  verify: go test ./tools/bind/ -run 'TestAutoPolicy|TestAutoRunCommands|TestAutoEligible|TestAutoEvidence|TestAutoBreaker|TestGuard'
- [ ] 偽の LLM コマンドで、抽出 → 逆変換 → 試し実行 → 承認 → 証拠の記録が通る
  verify: go test ./pkg/services/ -run TestAutoApprove
- [ ] 環境変数の LLM コマンドが自動承認に使われない
  verify: go test ./pkg/services/ -run TestAutoApproveIgnoresEnvCommand
- [ ] 証拠の無い・書き換えられた自動承認は unapproved になる
  verify: go test ./pkg/services/ -run TestCheckAutoEvidence
- [ ] spec-gate guard が hook の入力を読み、拒否なら exit 2、許可なら exit 0 で終わる
  verify: go test ./pkg/interfaces/cli/ -run TestGuardHook
- [ ] 承認役のサブエージェントがあり、frontmatter の PreToolUse hook で spec-gate guard を呼ぶ
  verify: test -f .claude/agents/spec-gate-approver.md && grep -q 'spec-gate guard' .claude/agents/spec-gate-approver.md
- [ ] 承認済みの claim（このプランのものを含む）がすべて blocking でない
  verify: go run . check --no-plans
- [ ] 全体のテストと lint が通る
  verify: go vet ./... && go test ./...

## 備考

- 証拠を作る LLM 呼び出しは `.spec-gate/policy.yaml` の `llm_commands` だけ。テストでは固定の回答を返すスクリプトを `llm_commands` にする
- `spec-gate guard` の hook を `.claude/settings.json` に足すのは人間（実装の最後に頼む）。足したあとは、エージェントは保護したパスを編集できない
- hook の入力の `agent_type` は実機で確かめる（verify にはできない）。実装後、人間に承認役を 1 回起動してもらい、拒否されないことと、
  メインのエージェントからの `auto-approve` が拒否されることを見てもらう
- 既存の `review-decisions.jsonl` の形（リモート承認・MCP Apps）は変えず、行を足す経路（CLI の approve・承認画面・auto・revoke）を増やす
- まとめ承認（`approve --pending`）の対象の条件と、自動承認の判定の 3〜6 は同じ考え方。判定の関数は共有する（depgraph の duplicate-code を出さない）
