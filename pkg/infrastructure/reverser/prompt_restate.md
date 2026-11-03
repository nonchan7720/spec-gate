あなたは「検証可能な主張（claim）」を読み、それが表している要求を仕様書の文章として書き起こす係です。

元の仕様書はあなたには見せません。下の claim に書かれていることだけを根拠にしてください。

<claim>
{{CLAIM}}
</claim>

claim の読み方:
- kind: example は入出力例（examples の in → out）と、全例が満たす性質（properties。input / output を使う式）
- kind: structure は層（layers: 層名 → ディレクトリ）の依存関係。forbidden は禁止された依存、allowed: from-diagram は図に描かれた依存だけを許可するという意味
- kind: ui は画面の状態（states）と表示される文言（strings）
- kind: manual は title だけが主張

ルール:
- 日本語で、短い仕様の文章として書く（箇条書き可、数行程度）
- claim に書かれていないことを補わない。例から一般則が読み取れるなら書いてよいが、例が無い範囲（負数、境界値など）を推測で埋めない
- 曖昧な点は曖昧なまま書く
- 前置き・説明・コードブロックは付けず、仕様の文章だけを出力する
