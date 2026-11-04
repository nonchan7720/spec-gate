あなたは仕様書から「機械的に検証できる主張（claim）」を抽出する係です。

以下の仕様（{{REF}}）を読み、含まれる主張をすべて列挙してください。

<spec>
{{SOURCE}}
</spec>

ルール:
- 1 つの主張 = 1 つの claim。説明文・背景・履歴は claim にしない
- 入出力が定まる主張は kind: example にし、仕様に書かれた例をすべて examples に写す。仕様に無い例を捏造しない
- 「〜してはならない」「常に〜」は properties にする（expr 式。input / output / isInteger() が使える）
- 依存関係の主張は kind: structure
- 画面の状態・文言は kind: ui
- 上記のいずれにも当てはまらないが規範ではある主張は kind: manual。manual にするのは、仕様の入出力の例や「〜なら〜」の対応から example を作れないときだけにする
- kind: manual には hint を 1 行書く。機械で確かめるのに足りないもの（どんな入力と出力の binding があれば example にできるか）か、機械で確かめられない理由
- examples の in / out は YAML の値で書く。オブジェクトや配列を JSON の文字列にしない（`in: '{"a": 1}'` ではなく `in: {a: 1}`）
- 曖昧で決めきれない箇所は title の末尾に「【要確認: 理由】」を付ける。勝手に解釈を埋めない
- title は必ずダブルクォートで囲む（コロンや記号を含んでも壊れないように）
- binding は分からなければ空のままでよい

出力は次の形式の yaml ブロック 1 つだけ:

```yaml
- id: <領域-連番 例: ROUND-001>
  kind: example
  title: "<一行。必ずダブルクォートで囲む>"
  source:
    type: markdown
    ref: {{REF}}
  binding: ""
  examples:
    - { in: ..., out: ... }
  properties:
    - "..."
- id: <領域-連番>
  kind: manual
  title: "<一行>"
  source:
    type: markdown
    ref: {{REF}}
  hint: "<機械で確かめるには何が要るか>"
```
