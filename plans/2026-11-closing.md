---
title: 月末締め処理の実装
status: draft        # active にすると check の対象になる
claims: [CLOSE-001, ROUND-001, ROUND-002]
---

# 月末締め処理の実装

## 目的

docs/spec/rounding.md の「月末処理」を実装し、CLOSE-001 を manual から example に昇格させる。

## チェック項目

- [ ] 丸め関数が仕様の例をすべて満たす
  verify: go run ./cmd/spec-gate check --only ROUND-001,ROUND-002
- [ ] 締め日算出関数が存在する
  verify: test -f src/closing/closingDate.ts
- [ ] 締め日算出のテストが通る
  verify: npx vitest run src/closing
- [ ] 層の依存ルールを壊していない
  verify: go run ./cmd/spec-gate check --only ARCH-001

## 備考

「休日でも前倒ししない」のでカレンダー連携は不要。月末日 = `new Date(y, m+1, 0)`。
