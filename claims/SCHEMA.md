# claim ファイルの書式

claim は「仕様のうち、機械的に検証できる形にした1つの主張」。
ファイル1つ = claim 1つ。`claims/<領域>/<ID>.yaml`。

## 共通フィールド

```yaml
id: ROUND-001            # 一意。領域プレフィックス + 連番
kind: example            # example | structure | ui | manual
title: 円未満は切り捨て   # 人間が読む一行
source:
  type: markdown         # markdown | mermaid | figma | image
  ref: docs/spec/rounding.md#端数処理   # 仕様の該当箇所
  drift: required        # required（既定）| optional。optional は snapshot が取れなくても fail にしない
approved:                # `gate approve <id>` が書く。手で書かない
  by: someone
  at: 2026-11-03T00:00:00Z
  source_hash: sha256:…  # 承認時点の仕様のハッシュ。変わると stale
```

`source.ref` の書き方:

| type | ref |
|---|---|
| markdown | `path/to/file.md#見出しテキスト`（見出し省略でファイル全体） |
| mermaid | `path/to/file.mmd` |
| figma | `figma://<fileKey>/<nodeId>` |
| image | `path/to/file.png` |

## kind: example — 入出力例と性質

```yaml
binding: npx tsx tools/call.ts src/payroll/round.ts roundYen   # 下記の契約を満たすコマンド
examples:
  - { in: 1234.56, out: 1234 }
  - { in: [1, 2], out: 3 }               # 引数が複数なら配列
  - { in: -0.5, out: 0, note: "マイナスは0" }
properties:                              # 全 example が満たすべき式（expr 構文）。input / output / isInteger() が使える
  - "output <= max(input, 0)"
  - "isInteger(output)"
```

**binding の契約**（言語非依存）: stdin に `{"in": <値>}` を受け取り、stdout に `{"out": <値>}` を 1 行で返す。
exit code 非 0 は失敗。TypeScript なら `tools/call.ts`、Go なら `go run ./tools/call -fn roundYen` のような薄いラッパを用意する。

**properties の構文**は [expr](https://expr-lang.org/)。`&&` `||` `!` `<=` `in` `len()` `max()` `min()` `abs()` などが使える。
JavaScript の `Math.` や `Number.` は使えない（`lint` が弾く）。

### fuzz / relations — ランダム入力による検証

examples の数件だけを満たす実装を落とすため、生成した入力でも properties と relations を評価できる。

```yaml
fuzz:
  runs: 200                # 生成する入力の数（1〜10000）
  seed: 42                 # 乱数の種（省略可。省略時は claim ID の 64 ビット FNV-1a ハッシュ）
  input:
    type: number           # number（実数）| integer（整数）
    min: -100000           # 下限（含む）
    max: 100000            # 上限（含む）。integer なら min / max も整数
relations:                 # メタモルフィック関係（expr 構文）。x は生成した入力、f(...) は binding の呼び出し
  - "f(x + 1) == f(x) + 1"
```

- `check` は examples がすべて一致したあとで、seed から `runs` 個の入力を決定的に生成して binding に流す。
  同じ seed なら同じ入力列になる
- 各入力 x について、出力が properties（`input` = x、`output` = binding の出力）を満たすか、
  relations が真になるかを評価する。最初に満たさない入力が見つかった時点で fail にし、
  反例 x と seed を出す（`seed:` にその値を書けば同じ入力列で再現できる）
- binding の結果は入力ごとに 1 回だけ求めて使い回す。呼び出しは最大で `runs` ×（1 + relations 中の `f(...)` の数）回
- relations は比較や論理演算で真偽を返す式にする（`f(x) + 1` のような式は `lint` が弾く）。`f` の引数は 1 つ
- relations を書くなら fuzz が必要。fuzz を書くなら properties か relations が必要。どちらも kind: example だけ

## kind: structure — 依存構造

```yaml
layers:                   # 層の名前 → ディレクトリ。名前は Mermaid のノードIDと一致させる
  app: src/app
  domain: src/domain
  infra: src/infra
allowed: from-diagram     # source の Mermaid に描かれたエッジだけを許可する
forbidden:                # 明示的に禁止する依存（allowed と併用可）
  - domain -> infra
```

## kind: ui — 画面

```yaml
story: src/ui/PaymentList.stories.tsx   # Figma の variant と同名の story が存在すること
states: [Default, Empty, Loading, Error]
url: http://localhost:6006/iframe.html?id=paymentlist--default   # --ui 指定時に Playwright で開く
strings:                                # 画面に表示されるべき文言（Figma のテキストノードから）
  - 支払一覧
```

## kind: manual — 機械判定できないもの

検証手段を持たない claim。`check` では常に `unverified` として報告される。
「見た目が正しい」「文章のトーンが合っている」などはここに置き、人間のレビュー対象として可視化する。

## `claims/_pending/`

`gate extract` が出力する候補置き場。承認されるまで check の対象にならない。
ファイルには `confidence: 2/3` のように合議の一致数が書かれる。
