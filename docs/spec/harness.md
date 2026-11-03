# spec-gate ハーネス拡張仕様

本書は spec-gate 自身に追加する機能（holdout・リテラル漏れ検知・ランダム検証・mutation score・
CI 出力・役割別レポート・再リンク提案・通り方のログ）の仕様を定める。
各節の「binding」は、その節の入出力例を検証するコマンドである（stdin に `{"in": ...}`、stdout に `{"out": ...}`）。

## 背景

AI エージェントは、仕様の例の値を実装に直接書き込む・例の数件だけ通る実装にする・検証を甘くする、といった
「通り方」で claim を満たそうとすることがある。本書の機能は、それを機械的に塞ぐか、少なくとも人間に見えるようにする。

## holdout examples

claim の examples とは別に、エージェントに見せない入出力例（holdout）を持てる。

- holdout は `holdout/<claim-id>.yaml` に置き、`examples:`（in / out の配列）だけを書く
- `check` は examples に続けて holdout も binding に流して比べる
- holdout が不一致でも、レポートとエージェントへの出力に holdout の入力・期待値・実際の値を出してはならない。件数だけを出す
- holdout が 1 件でも不一致なら、その claim は fail
- `lint` は holdout も examples と同じく properties との整合を確かめる

binding: `go run ./tools/bind holdout-summary`（入力は holdout の判定結果の配列、出力はレポートに出す 1 行）

| 入力 | 出力 |
|---|---|
| `[{"pass": true}, {"pass": true}]` | `"holdout 2 件すべて一致"` |
| `[{"pass": true}, {"pass": false, "in": 1.5, "out": 1, "got": 2}]` | `"holdout 2 件中 1 件が不一致（値は表示しない）"` |
| `[]` | `"holdout なし"` |

## holdout の読み取り拒否

エージェントが holdout を読めると意味がないので、hook-gate と Claude Code の権限で読み取りを拒否する。

- `.claude/settings.json` の `permissions.deny` に `Read(./holdout/**)` を入れる
- `holdout/` を読む・表示するシェルコマンド（cat / less / head / tail / grep / git show / git log -p など）を hook-gate の `pre_bash` で拒否する
- `holdout/` への書き込み（Edit / Write、シェルからの書き込み）も `claims/` と同じく拒否する
- spec-gate 本体（`check` / `lint`）は holdout を読める

## リテラル漏れ検知

実装ファイルに、claim の examples・holdout の値がそのまま書き込まれていないかを検知する。

- `spec-gate leak <file>...` は、指定ファイルの内容に examples・holdout の in / out の値が現れるかを調べ、見つかれば値と行番号を出して exit 1
- 数値は「数字が 3 桁以上」のものだけを対象にする（0・1・10 のようなありふれた値で誤検知しない）。桁は符号と小数点を除いた数字（0〜9）の個数で数える（1.25 は 3 桁、-12 は 2 桁）
- 数値は、前後が数字・小数点・英字でない位置に、その値の表記がそのまま現れたときだけ検知する（1234.56 の中の 1234 は 1234 として数えない）
- 文字列は 4 文字以上のものだけを対象にする
- テストファイル（`_test.go`、`.test.ts`、`.spec.ts`）と `claims/`・`holdout/`・`docs/` は対象外
- hook-gate の `post_edit` から実装ファイルの編集ごとに呼び、見つかったら警告する

binding: `go run ./tools/bind leak`（入力は `{"code": 対象の内容, "values": 値の配列}`、出力は見つかった値の配列）

| 入力 | 出力 |
|---|---|
| `{"code": "if x == 1234.56 { return 1234 }", "values": [1234.56, 1234]}` | `[1234.56, 1234]` |
| `{"code": "return math.Floor(x)", "values": [1234.56, 1234]}` | `[]` |
| `{"code": "return 0", "values": [0, 1]}` | `[]` |
| `{"code": "msg := \"円未満は切り捨て\"", "values": ["円未満は切り捨て", "ok"]}` | `["円未満は切り捨て"]` |

## ランダム入力による検証

examples の数件だけを満たす実装を落とすため、properties をランダムな入力で検証できる。

- claim に `fuzz:` を書くと、`check` はその回数だけ入力を生成して binding に流し、properties を評価する
- `fuzz:` には回数（`runs`）と入力の範囲（`input.type`: number / integer、`input.min`、`input.max`）を書く
- `input.min` と `input.max` は範囲の両端を含む
- 入力の生成は乱数の種（`seed`）で決まり、同じ種なら同じ入力列になる。種を省略したら claim ID の FNV-1a（64 bit）を種にする
- properties を満たさない入力が見つかったら fail にし、その入力（反例）を出す
- メタモルフィック関係（例: `f(x + 1) == f(x) + 1`）を `relations:` に書ける。`f` は binding の呼び出しを表し、生成した各入力 x について評価する

binding: `go run ./tools/bind relation`（入力は `{"relation": 式, "values": {"f(x)": 値, ...}, "x": 値}`、出力は真偽）

| 入力 | 出力 |
|---|---|
| `{"relation": "f(x + 1) == f(x) + 1", "x": 1.5, "values": {"f(x)": 1, "f(x + 1)": 2}}` | `true` |
| `{"relation": "f(x + 1) == f(x) + 1", "x": 1.5, "values": {"f(x)": 1, "f(x + 1)": 3}}` | `false` |

## mutation score

プランの verify が甘くないかを、凍結前にコードをわざと壊して確かめる。

- `spec-gate plan mutate <plans/x.md>` は、プランの claims が触るファイル（binding のパス・layers・story）に変異を 1 か所ずつ入れて verify を実行し、落ちた変異の割合（mutation score）を出す
- 変異を入れたファイルは、成功・失敗・中断に関わらず必ず元に戻す
- プランの front matter に `mutation: 0.8` のように閾値を書くと、`plan freeze` は score が閾値未満なら凍結を拒否する
- 変異の種類は次の一覧で全部とする（1 回に 1 か所だけ変える。文字列リテラルとコメントの中は変えない）
  - 算術: `+` → `-`、`-` → `+`、`*` → `/`、`/` → `*`
  - 比較: `>` → `>=` と `<`、`<` → `<=` と `>`、`>=` → `>`、`<=` → `<`、`==` → `!=`、`!=` → `==`
  - 整数リテラル: 2 以上の整数 n → n+1（0 と 1 は添字や真偽に使われやすいので変えない）
- 変異が 1 件も作れないときは score を出さず、閾値が書かれたプランの凍結は拒否する（検証できないものを通さない）

binding: `go run ./tools/bind mutate`（入力はコード 1 行、出力はその行に 1 か所だけ変異を入れた行の配列）

| 入力 | 出力 |
|---|---|
| `"a + b"` | `["a - b"]` |
| `"a * b"` | `["a / b"]` |
| `"x > 0"` | `["x >= 0", "x < 0"]` |
| `"x == y"` | `["x != y"]` |
| `"return 100"` | `["return 101"]` |
| `"return x"` | `[]` |

## CI 用の出力と指標

hook ではなく CI から使うための出力を持つ。

- `check --json` は判定結果を JSON で出す。各 claim の id・status・title・detail・spec と、件数の集計を含む
- `--json` でも exit code は通常の `check` と同じ（blocking があれば 1）
- `spec-gate metrics` は claim の kind 別件数と割合、unverified 率を出す。`--json` で JSON にする
- unverified 率は「unverified の件数 / claim の総数」で、claim が 0 件なら 0 とする

binding: `go run ./tools/bind unverified-rate`（入力は status の配列、出力は割合）

| 入力 | 出力 |
|---|---|
| `["pass", "pass", "unverified", "unverified", "fail"]` | `0.4` |
| `[]` | `0` |
| `["unverified"]` | `1` |

## 役割別レポート

見る人ごとに、その人が対応すべきものだけを出す。

- `check --role engineer` は fail だけを、仕様の位置（file:line）つきで出す
- `check --role designer` は kind: ui の claim のうち unverified と fail を出す
- `check --role pm` は stale と unapproved を出す
- `--role` を付けても exit code は通常の `check` と同じ

binding: `go run ./tools/bind role-filter`（入力は `{"role": 役割, "results": [{"id", "kind", "status"}]}`、出力は出す claim の id の配列）

| 入力 | 出力 |
|---|---|
| `{"role": "engineer", "results": [{"id": "A", "kind": "example", "status": "fail"}, {"id": "B", "kind": "example", "status": "pass"}]}` | `["A"]` |
| `{"role": "designer", "results": [{"id": "U", "kind": "ui", "status": "unverified"}, {"id": "E", "kind": "example", "status": "unverified"}]}` | `["U"]` |
| `{"role": "pm", "results": [{"id": "S", "kind": "example", "status": "stale"}, {"id": "N", "kind": "manual", "status": "unapproved"}, {"id": "F", "kind": "example", "status": "fail"}]}` | `["S", "N"]` |

## 曖昧の記録

承認 UI で、claim の誤りではなく「仕様が決まっていない」ことを記録できる。

- 承認 UI の各候補に「曖昧」ボタンを置く。押すと理由を入力させ、候補を `claims/_ambiguous/` に移して理由を `# ambiguous:` ヘッダに記録する
- `claims/_ambiguous/` は `_pending/` と同じく check の対象外
- `coverage` は、曖昧として記録された claim の参照する単位を「要確認」として出す

## 見出し変更時の再リンク提案

仕様の見出しが変わって claim の参照先が見つからないとき、近い見出しを提案する。

- claim の `source.ref` の見出しが仕様に無いとき、`check` の fail の説明に、最も近い見出しを「もしかして」として添える
- 近さは見出しの文字列の編集距離（文字単位）で測り、距離が見つからない見出しの文字数の半分以下のものだけを提案する
- 距離が同じ候補が複数あるときは、仕様の中で先に現れる見出しを提案する
- 提案だけで、claim は書き換えない（claim の修正は人間が行う）

binding: `go run ./tools/bind relink`（入力は `{"missing": 見出し, "headings": 見出しの配列}`、出力は提案する見出し、無ければ null）

| 入力 | 出力 |
|---|---|
| `{"missing": "端数処理", "headings": ["端数の処理", "マイナス金額"]}` | `"端数の処理"` |
| `{"missing": "端数処理", "headings": ["背景", "変更履歴"]}` | `null` |

## 変更検知の精度

`check --changed` は、変更されたファイルに関係する claim だけを判定する。見逃しを避ける側に倒す。

- binding の中の語のうち、リポジトリ内に実在するファイルまたはディレクトリを指すものを、その claim が触るパスとする
- 変更されたファイルが、claim が触るファイルそのもの、または claim が触るディレクトリの配下なら、その claim は対象
- 変更されたファイルが、claim が触るファイルと同じディレクトリにあり、拡張子が同じなら対象（同じパッケージの別ファイルの変更を見逃さない）
- claim が触るパスがこのモジュール内の Go パッケージのディレクトリ（または Go ファイル）なら、そのパッケージが直接・間接に import するモジュール内のパッケージのディレクトリも触るパスに加え、上の 3 つの規則で比べる（`go run ./tools/bind relink` の claim は、呼び出す先の実装の変更でも対象になる）。依存を調べられないときは加えるパスがないだけで、元の触るパスは外さない

binding: `go run ./tools/bind touched`（入力は `{"touches": パスの配列, "changed": 変更ファイル}`、出力は対象かどうか。touches は依存パッケージを加え終えたパスの配列）

| 入力 | 出力 |
|---|---|
| `{"touches": ["src/payroll/round.ts"], "changed": "src/payroll/round.ts"}` | `true` |
| `{"touches": ["src/payroll/round.ts"], "changed": "src/payroll/util.ts"}` | `true` |
| `{"touches": ["src/payroll/round.ts"], "changed": "src/payroll/README.md"}` | `false` |
| `{"touches": ["src/app"], "changed": "src/app/handler/x.ts"}` | `true` |
| `{"touches": ["src/payroll/round.ts"], "changed": "src/ui/List.tsx"}` | `false` |
| `{"touches": ["tools/bind", "pkg/domain"], "changed": "pkg/domain/relink.go"}` | `true` |

## 通り方のログ

claim がどう通ったかを記録し、無理やり通した形跡を人間が見られるようにする。

- `check --hook` を実行するたびに、claim ごとの判定を `.spec-gate/trace.jsonl` に 1 行ずつ追記する（時刻・claim id・status・その時点の変更行数）
- 変更行数は `git diff --numstat HEAD` の追加行と削除行の合計とする。git リポジトリでなければ 0
- `spec-gate trace` は claim ごとに、最後に pass になったときについて、その直前の連続 fail 回数と、その pass の時点の変更行数を出す
- `.spec-gate/trace.jsonl` は git の管理対象外にする

binding: `go run ./tools/bind trace-summary`（入力は 1 つの claim の判定履歴（古い順）、出力は `{"fails_before_pass": 回数, "lines_at_pass": 行数}`。pass していなければ null）

| 入力 | 出力 |
|---|---|
| `[{"status": "fail", "lines": 10}, {"status": "fail", "lines": 40}, {"status": "pass", "lines": 120}]` | `{"fails_before_pass": 2, "lines_at_pass": 120}` |
| `[{"status": "pass", "lines": 5}]` | `{"fails_before_pass": 0, "lines_at_pass": 5}` |
| `[{"status": "fail", "lines": 3}]` | `null` |

## LLM 処理のサブエージェント委譲

`extract`（合議抽出）と `reverse`（逆変換）の LLM 処理は、spec-gate を動かしているエージェントのサブエージェントに任せる。
spec-gate 自身は LLM を呼ばず、プロンプトの書き出し（emit）と、回答の取り込み（collect）だけを行う。

- `spec-gate extract <ref> --runs N --emit` は、抽出のプロンプトを N 個書き出し、そのパスを表示する。LLM は呼ばない
- `spec-gate extract <ref> --collect` は、N 個の回答を読み、これまでと同じ合議で `claims/_pending/` に置く。回答が欠けていれば、欠けている run を表示してエラーにする
- `spec-gate reverse [<id>...] --emit` は、claim ごとに再生成のプロンプトを書き出す。`--collect` は、再生成の回答がそろっていれば比較のプロンプトを書き出し、比較の回答もそろっていればレポートを出す
- プロンプトと回答は作業ディレクトリ `.spec-gate-llm/` に置く（`.spec-gate/` はエージェントの書き込みを禁止しているので使わない）。git の管理対象外にし、collect が終わったら消す
- 回答は、各プロンプトと同じ場所の `run-<k>.answer.md`（reverse は `<claim-id>.restate.answer.md` / `<claim-id>.compare.answer.md`）に書く
- 各サブエージェントは自分のプロンプトだけを読み、他の run の回答を読まない（合議の独立性）。サブエージェントの定義（`.claude/agents/`）と、emit → 並列起動 → collect の手順（`.claude/skills/`）を同梱する
- 環境変数 `SPEC_GATE_EXTRACT_CMD` / `SPEC_GATE_REVERSE_CMD` を設定したときだけ、これまでどおりコマンド（`claude -p` など）を呼ぶ。設定せず `--emit` / `--collect` も付けなければ、委譲の手順を案内してエラーにする（意図せず LLM を呼ばない）

作業ディレクトリ内のパスは、ref の `/`・`#`・空白を `_` に置き換えた名前を使う。

binding: `go run ./tools/bind llm-paths`（入力は `{"ref": ref, "runs": 回数}`、出力は extract のプロンプトのパスの配列）

| 入力 | 出力 |
|---|---|
| `{"ref": "docs/spec/rounding.md#端数処理", "runs": 2}` | `[".spec-gate-llm/extract/docs_spec_rounding.md_端数処理/run-1.prompt.md", ".spec-gate-llm/extract/docs_spec_rounding.md_端数処理/run-2.prompt.md"]` |
| `{"ref": "docs/arch/layers.mmd", "runs": 1}` | `[".spec-gate-llm/extract/docs_arch_layers.mmd/run-1.prompt.md"]` |

## 変更履歴

- 2026-11-04 初版
- 2026-11-04 LLM 処理のサブエージェント委譲を追加
- 2026-11-04 合議抽出で【要確認】になった箇所（漏れ検知の桁数、乱数の種と範囲、変異の一覧、再リンクの長さと同点、変更行数と pass の基準）を確定
- 2026-11-04 変更検知の精度に、触る Go パッケージが import するモジュール内のパッケージも触るパスに加える規則を追加
