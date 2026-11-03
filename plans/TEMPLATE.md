---
title: <プラン名>
status: draft        # draft | active | done
claims: []           # このプランが満たすべき claim の ID
---

# <プラン名>

## 目的

<何のために何をするか>

## チェック項目

すべての項目に `verify:` を付ける。コマンドの exit code 0 を「完了」とみなす。
`verify:` のない項目があると `gate plan freeze` に失敗する。

- [ ] <やること>
  verify: <コマンド>
- [ ] <やること>
  verify: <コマンド>

## 備考

<設計判断、代替案、懸念>
