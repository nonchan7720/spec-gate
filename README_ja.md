[English](README.md) | 日本語

# spec-gate — 仕様準拠ハーネス

```
PJ内のコーディングルール   → hook-gate
コードの負債を溜めない     → depgraph
プロダクト品質の担保       → spec-gate ← これ
```

自然言語の仕様（Markdown / Mermaid / Figma / 画像）と、エージェントと作ったプランファイルが
**AI の書いたコードで守られているか**を、エージェントの自己申告を一切使わず exit code で判定する。

設計思想は 1 つ：**AI は嘘をつく前提で、判定を機械に寄せる。モデルが賢くなればハーネスは素通りになり、往復が減る。**

## 仕組み

```
source (何でもいい)  →  adapter  →  claim (構造化)  →  check (exit code)
Markdown / Mermaid /                 YAML 1主張1ファイル    example / structure / ui / plan
Figma / PNG / PDF                    ↑ 人間承認はここ 1 回だけ
```

自然言語とコードを直接照合はしない。間に **claim（検証可能な主張）** を挟む。

| 工程 | コマンド | 誰が | 何が起きる |
|---|---|---|---|
| 抽出 | `spec-gate extract <ref> --runs 3` | AI | 同じ仕様を独立に N 回読み、一致した主張だけ高信頼にして `claims/_pending/` へ（合議） |
| 整合 | `spec-gate lint` | 機械 | examples が properties を満たすか、claim 同士が矛盾しないか |
| 承認 | `spec-gate approve <id>` | **人間** | 仕様の該当箇所を表示し、ハッシュを凍結。lint 失敗なら拒否 |
| 判定 | `spec-gate check` | 機械 | 承認 → 仕様ドリフト → 種類別チェック → プランの verify |
| 漏れ | `spec-gate coverage` | 機械 | 仕様のうち claim に紐づかない箇所を列挙 |

### 判定の順序と状態

`check` は claim ごとに次の順で見て、最初に引っかかった段階で止まる。

```
unapproved  承認がない                        → blocking
stale       仕様が承認後に変わった            → blocking（人間が再承認するまで解けない）
fail        コードが claim を満たさない       → blocking（エージェントが直す）
unverified  検証手段がない（manual / --ui 無し / depgraph が判定できない）→ 人間のレビュー対象。エージェントは止めない
pass
```

blocking があれば exit 1。`--hook` では stderr に直すべきものだけのレポートを出して exit 2。

### 嘘をつけない構造

- **claim は AI が触れない**。`claims/**` と `.spec-gate/` の編集、`approve` / `plan freeze` の実行を
  hook-gate の `pre_edit` / `pre_bash` ルールで拒否する（`.claude/feedback/protect_spec_claims.md`）
- **プランの verify は先に凍結する**。`spec-gate plan freeze` で verify 行のハッシュを記録し、
  実装後に verify を緩めると `stale` になる。加えて、プランを編集した直後に hook-gate の `post_edit` が
  `spec-gate plan check` を走らせ、凍結済みの verify が変わっていればその場で止める
  （`.claude/feedback/protect_plan_verify.md`）
- **仕様が変わったら自動で stale**。仕様側を書き換えて辻褄を合わせることもできない
- **検証手段のない claim は fail ではなく unverified**。「検証していないのに通った」を作らない代わりに、
  人間が見るべきものとして可視化する

## インストール

**GitHub Releases（推奨）。** [Releases](https://github.com/nonchan7720/spec-gate/releases) から OS / CPU に合うアーカイブを取得する。
`spec-gate_<os>_<arch>.tar.gz`（Windows は `.zip`）で、linux / darwin / windows × amd64 / arm64 がある。
`checksums.txt` で検証してから `spec-gate` を `PATH` に置く。どのアーカイブにも `LICENSE`・`NOTICE`・
`THIRD_PARTY_NOTICES.md` が入っている。

```bash
sha256sum --ignore-missing -c checksums.txt   # macOS: shasum -a 256 --ignore-missing -c checksums.txt
tar xzf spec-gate_linux_amd64.tar.gz && install spec-gate ~/.local/bin/
spec-gate --version
```

`v<version>-dev.<commit>` という名前のリリースは、`main` への push ごとに作られるプレリリース。

**Go で入れる。**

```bash
go install github.com/nonchan7720/spec-gate@latest
```

**ソースから。** リポジトリを clone し、`mise.toml` で固定したツールチェーンでビルドする：

```bash
git clone https://github.com/nonchan7720/spec-gate.git && cd spec-gate
mise run setup   # mise install、go mod download、サンプルアプリの npm 依存、bin/spec-gate
```

spec-gate は pure Go（`CGO_ENABLED=0`）なので、素の `go build` でクロスコンパイルできる。

## 動かす

```sh
go build -o spec-gate .                    # or: go run .
pnpm install --frozen-lockfile             # サンプルの TS 側（tools/call.ts 用）

./spec-gate status                         # claim とプランの一覧
./spec-gate lint                           # 内部整合
./spec-gate approve ROUND-001 ROUND-002 CLOSE-001 ARCH-001 UI-001
./spec-gate check                          # 全判定
./spec-gate coverage                       # 抜け落ち
./spec-gate plan check plans/x.md          # 凍結済みプランの verify が変わっていないか
```

サンプルは承認済みの状態で同梱してあるので、clone 直後の `check` は通る。
壊してみると分かりやすい：

```sh
sed -i 's/Math.floor/Math.round/' src/payroll/round.ts && ./spec-gate check --only ROUND-001
#  ✗ ROUND-001  fail  円未満は切り捨て  [docs/spec/rounding.md:10]
#        examples[0]: in=1234.56 期待=1234 実際=1235

sed -i '/infra: \[app\]/d' .depgraph.yml && ./spec-gate check --only ARCH-001
#  ✗ ARCH-001  fail  図（仕様）と .depgraph.yml が一致しない。.depgraph.yml を図に合わせること:
#        infra -> app は仕様で許可されていないが forbidden.infra に app が無い

sed -i 's/切り捨てる/四捨五入する/' docs/spec/rounding.md && ./spec-gate check --only ROUND-001
#  ⟳ ROUND-001  stale  仕様が承認後に変更された。内容を確認して approve で再承認する
```

### Claude Code との接続

判定は hook-gate（feedback-gate）の `.claude/gate.yaml` から走る。独自の Stop hook は持たない。

- 編集のたびに、変更したファイルに応じて gofmt / go vet / go test（Go）、`depgraph gate`（Go / TS / JS）、
  `spec-gate lint`（claims）、`spec-gate check --changed`（仕様・プラン・サンプルアプリ）が走る
- 終了時に `consistency_checks` の `go-all`（build / golangci-lint / 全テスト）と
  `spec-gate`（`check --changed --hook`）が走り、失敗があれば終了をブロックする。
  連続失敗で諦める処理（5 回）は hook-gate 側が持つ

depgraph は `docker compose up -d` で起動する Neo4j（`localhost:17687`）を使う。接続先は `mise.toml` の `[env]` にあり、
depgraph は hook・gate とも `mise exec --` 経由で呼ぶ。`.claude/settings.json` の SessionStart hook（`depgraph hook session-start`）が
セッション開始時の基準を記録し、`depgraph gate` はそこから増えた負債だけで失敗する。

`CLAUDE.md` にエージェント向けのルールがある。

## ディレクトリ

```
docs/spec/*.md        詳細仕様（Markdown）          ─┐
docs/arch/*.mmd       アーキテクチャ図（Mermaid）    ├ source
Figma / *.png / *.pdf  UI・その他                   ─┘
sources.yaml          監視する source の登録簿（coverage 用）
claims/<領域>/*.yaml  claim。書式は claims/SCHEMA.md
claims/_pending/      extract の出力。承認まで check 対象外
plans/*.md            プラン。front matter の status: active で check 対象
.spec-gate/           プランのロック（コミット対象）
.depgraph.yml         depgraph のレイヤー定義。図と一致していることを structure claim が照合する
.claude/gate.yaml     hook-gate の品質ゲート
.claude/feedback/     hook-gate の保護ルール（claims・verify 行）
tools/call.ts         example claim の binding 用ラッパ（TS 関数向け）

main.go               エントリポイント
pkg/domain            Claim / Plan / Result、ポート、lint（純粋）
pkg/services          check / approve / coverage / extract / plan
pkg/infrastructure    claimrepo, adapter(markdown/mermaid/figma/image), checker, planfile, vcs, extractor
pkg/interfaces/cli    cobra コマンド、レポート、hook 出力
```

## claim の種類と、何が機械的に守れるか

| kind | source の例 | 検証 | 確度 |
|---|---|---|---|
| `example` | 詳細仕様（端数・締め日） | binding コマンドに入出力例を流し、properties（expr 式）も評価 | 高・決定的 |
| `structure` | Mermaid の図 | 図のエッジ（= 許可）と `.depgraph.yml` の layers / forbidden が一致するかを照合し、依存違反の検出は `depgraph gate` に任せる | 照合は決定的。検出は depgraph 次第 |
| `ui` | Figma | story に variant と同名の export があるか / `--ui` で Playwright が文言を確認 | 中 |
| `manual` | 何でも | なし。常に unverified | — |

`example` の binding は「stdin JSON → stdout JSON」の契約を満たすコマンドなら何でもよいので、
TS / Go / Python どれでも同じ claim で検証できる。

### structure と depgraph

図 = 仕様、depgraph = 検出器、spec-gate = 両者の整合。

1. claim の各層が `.depgraph.yml` の `layers` に同名であり、同じディレクトリ（`<dir>/**`）を指すこと
2. claim の層の全順序対 A→B について、図で許可された依存（claim の `forbidden` を除く）は depgraph が禁止しておらず、
   許可されていない依存は depgraph が禁止していること
3. 一致していれば `depgraph scan --root . --claude-md ''`（`SPEC_GATE_DEPGRAPH_CMD` で差し替え可）で今のツリーを全件スキャンし、
   claim の層どうしの `layer-violation` が 1 件でもあれば fail（セッション前からある違反も含む。CLAUDE.md は書き換えない）。
   depgraph が未インストール、Neo4j に届かない、スキャンが完了しないときは unverified

### source adapter

| type | ref | ハッシュの取り方 | coverage の単位 |
|---|---|---|---|
| markdown | `path.md#見出し` | 節の本文（空白は正規化） | 見出し |
| mermaid | `path.mmd` | エッジ集合（ラベル・配置は無視） | エッジ |
| figma | `figma://<fileKey>/<nodeId>` | ノード部分木（座標は除外）。要 `FIGMA_TOKEN` | テキストノード・variant |
| image | `path.png` | ファイルハッシュ | ファイル |

Figma / 画像のように AI が直接読めない source は、`extract` の精度は保証しない。
人間が承認する工程があるので、候補の質が低くても「間違いが混入する」ことはない。

## UI 層の扱い

「見た目が正しい」は機械判定できない。できるものだけ切り出す：

- **決定的**: 文言（Figma のテキストノード = 画面の文字列）、状態の存在（variant = story）、デザイントークン名、画面遷移、a11y
- **近似**: レイアウト（スクリーンショットの構造差分、閾値付き）— 未実装
- **できない**: 「なんか違う」→ `manual` にして unverified で可視化

デザイナーの確認対象を「全画面」から「unverified だけ」に減らすのが狙い。

## 非エンジニア（PdM・デザイナー）との接点

彼らに YAML は見せない。接点は **承認** だけ：

```
docs/spec/rounding.md#端数処理 から読み取った主張
  ☑ 円未満は切り捨て
  ☑ マイナス金額は 0 にする
  ☐ 月次締めは月末日に実行 【要確認: 営業日の定義】
```

`extract` の出力をこの形で見せ、チェックを入れる = `approve`。「要確認」は仕様が決まっていない
ことの発見なので、それ自体が価値になる。承認 UI はこの雛形には含まれていない（`_pending/` を
読んで並べるだけなので薄い Web でも Slack でもよい）。

## 設計上の制約と未実装

- **claim が正しいかは最終的に人間が決める**。合議・lint・coverage は「間違いを人間の前で減らす」
  工夫であって保証ではない。本番でバグが出たら claim に遡って直す（claim の誤り / 欠落 / check の弱さ）
- `structure` の依存違反の検出は depgraph の全件スキャンに依存する。claim の層以外（循環・重複など）の負債は
  gate の `depgraph gate`（セッションで増えた分だけ）と depgraph の UI で見る
- `ui` の strings 検証は Storybook/アプリが起動している前提で `--ui` を付ける。CI でやるなら別ジョブ
- `extract` は `claude -p` を前提にしている。`SPEC_GATE_EXTRACT_CMD` で差し替え可
- 逆変換（claim → 仕様文を再生成して原文と diff）は未実装。合議と lint の次に入れる候補
- Miro は書き出し画像頼みになるので最も弱い。Mermaid 書き出しを運用で推奨した方がよい

## hook-gate / depgraph との役割分担

- hook-gate: 歩き方。Stop / PostToolUse / PreToolUse の実行基盤、claims と verify 行の保護
- depgraph: 依存の検出器。循環・レイヤー違反・重複
- spec-gate: 「何を作るか」が守られているか。example / ui / plan が本体。structure は図と depgraph 設定の整合のみ

## 開発

ツールチェーン（Go・golangci-lint・node・pnpm・goreleaser・actionlint）は `mise.toml` で固定している。

```bash
mise run setup            # 初回
mise run ci               # CI と同じ: vet, lint, lint:actions, test, build, licenses:check, release:check, spec:check
mise run release:snapshot # リリース用アーカイブを公開せずに dist/ へビルド
```

PR のタイトルは [Conventional Commits](https://www.conventionalcommits.org/ja/)（`feat: ...`、`fix: ...`）にする。
squash マージのコミットは PR タイトルになり、release-please はそこから CHANGELOG と次のバージョンを決める。

## ライセンス

Apache-2.0（[LICENSE](LICENSE) と [NOTICE](NOTICE) を参照）。バイナリに含まれる Go モジュールのライセンスは
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) にある。
