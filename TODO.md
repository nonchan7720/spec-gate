# TODO — エージェントの作業記録と残課題

HANDOFF.md は人間の引き継ぎ文書なので触らず、エージェント側の追記はここに書く。

## 2026-11-04 にやったこと

### HANDOFF「最初に直すこと 1」: Stop hook を hook-gate へ移設 — 完了

- `.claude/settings.json` の Stop hook を削除（中身は `{}`）
- `.claude/gate.yaml` を新規作成
  - rules: Go（パッケージ単位で gofmt / go vet / go test）、ルート `*.go` の gofmt、`go.mod` 系の `go mod tidy -diff`、
    `claims/**` の `spec-gate lint`、仕様・プラン・サンプルアプリ・`.depgraph.yml` の `spec-gate check --changed`
  - consistency_checks: `go-all`（go build / golangci-lint / go test ./...）、`spec-gate`（`go run . check --changed --hook`）
  - golangci-lint は `"$(mise where golangci-lint)/bin/golangci-lint"` で mise の 2.15.0 を明示
    （`~/.local/share/mise/installs/go/1.27.3/bin/` に v1.64.8 が残っていて PATH で先に解決され、typecheck が誤検出するため）
- spec-gate 側の 3 回カウンタ（`.spec-gate/hook-attempts`）を廃止。ループ防止は hook-gate（5 回）に一本化
  - `hookResult()` を純粋関数に切り出してテスト（`pkg/interfaces/cli/check_test.go`）
  - `.gitignore` から `.spec-gate/hook-attempts` を削除、`.claude/.gate-status/` を追加
- 保護ルール（hook-gate の feedback ルール）
  - `.claude/feedback/protect_spec_claims.md`: `claims/**`（`_pending/` 除く）と `.spec-gate/**` の Edit/Write を deny、
    シェル経由の書き換え（sed -i / リダイレクト / mv / cp / rm / git checkout 等）を deny、`approve` / `plan freeze` の実行を deny
  - `.claude/feedback/protect_plan_verify.md`: `plans/*.md` 編集後に `spec-gate plan check` を走らせ、凍結済み verify の変更を block
- `spec-gate plan check <plans/x.md>...` を追加（`PlanService.CheckFrozen`、テスト `pkg/services/plan_test.go`）

### HANDOFF「最初に直すこと 2」: structure checker を depgraph 整合チェックに置き換え — 完了

- `pkg/infrastructure/checker/structure.go`（自前の import 走査）を削除
- `pkg/infrastructure/checker/depgraph.go` を追加（テスト `depgraph_test.go` 12 件）
  1. claim の各層が `.depgraph.yml` の `layers` に同名であり、`<dir>/**` を指すこと
  2. claim の層の全順序対 A→B で「図で許可（claim の forbidden を除く）⇔ depgraph が禁止していない」こと
  3. 一致していれば `depgraph scan --root . --claude-md ''`（`SPEC_GATE_DEPGRAPH_CMD` で差し替え可）で全件スキャンし、
     claim の層どうしの layer-violation があれば fail（既存の違反も含む）。未インストール（exit 127）・
     scan の非0（Neo4j に届かない等）・スキャン完了表示なし → unverified
- `.depgraph.yml` を新規作成: spec-gate 本体（`core-domain` / `core-services` / `core-infrastructure` / `core-interfaces`）と
  サンプルアプリ（`app` / `domain` / `infra`、ARCH-001 の照合対象）の両方
- depgraph のグラフはこの PJ の `docker-compose.yml` の Neo4j（`spec-gate-neo4j`、bolt `localhost:17687`）に保存。
  `docker compose up -d` で起動してから `depgraph scan`。層違反は 0 件
  - `container_name` が depgraph 側の `depgraph-neo4j` と衝突していたので `spec-gate-neo4j` に変更
  - 接続先は `mise.toml` の `[env]` に一本化。depgraph は hook（settings.json）・gate（gate.yaml）とも `mise exec --` 経由で呼ぶ。
    structure claim が内部で depgraph を呼ぶので、`spec-check` も `mise exec -- go run . check ...` にしている
  - 最初に誤って depgraph 側の共有 Neo4j（7687）に作ったプロジェクト `spec-gate` は削除済み
- gate.yaml に `depgraph gate` のルールを追加（Go / TS / JS / `.depgraph.yml`）。当初は ARCH-001 経由でしか呼ばれておらず、
  Go 本体の編集では走っていなかった（指摘を受けて修正）
- `.claude/settings.json` に depgraph の SessionStart（基準の記録）と UserPromptSubmit（所見の注入）hook を追加

### その他

- 既存ツリーの赤を解消: gofmt 未適用 3 ファイル、golangci-lint（errcheck 8 件、unused 2 件）、go.sum の取り残し
- `./cmd/spec-gate` が存在しない（main.go はルート直下）ため、CLAUDE.md・package.json の参照を `go run .` に統一
- README を README.md（英語）/ README_ja.md（日本語）に分割し、上記の変更を反映

## 人間の作業が必要なもの

- [ ] `plans/2026-11-closing.md` の verify 行が `go run ./cmd/spec-gate ...` のまま（凍結済みなのでエージェントは変更不可）。
      `go run . ...` に直して `spec-gate plan freeze plans/2026-11-closing.md` で再凍結する。
      このままだと `status: active` にした時点で該当項目が fail する
- [ ] HANDOFF.md の記述が実装とずれている箇所（エージェントは編集しない）
  - 決定表「Stop hook は 3 回連続失敗で諦めて通す」→ hook-gate に移設・カウンタ廃止
  - 構成「`cmd/spec-gate/main.go`」「checker/ structure（import 走査）」、動作確認「`go build -o spec-gate ./cmd/spec-gate`」
  - 未実装「structure: tsconfig paths 未対応」「hook-gate 連携: 未着手」、次にやるなら 1・4

## 判断が必要な課題

- [x] **structure が既存の層違反を検出できない**（2026-11-04 修正）。`depgraph gate` をやめ、`depgraph scan --claude-md ''` の
      全件スキャンから claim の層どうしの layer-violation を拾うようにした。既存の違反を仕込んだコピーで fail になることを確認
- [x] `protect_spec_claims` の `pre_bash` 正規表現が、コマンド文字列中の文言にも反応していた（PR #2 で修正。
      書き込みはリダイレクト先・同じ行の引数だけ、approve / plan freeze はコマンド位置での実行だけを見る）
- [x] hook モードで spec-gate 自体がエラーになったとき（`check` の内部エラー）に exit 0 で通していた（2026-11-04 修正）。
      理由付きで exit 2 にした。止め続けるループは gate 側（5 回）が打ち切る

## 外部ツールの不具合（このリポジトリ外）

- [ ] feedback-gate: プロジェクト外のファイル（スクラッチパッドの `../../…/x.mjs`）が `**/*.mjs` の rule にマッチし、
      `CLAUDE_GATE_FILES` に入る
- [ ] depgraph gate: 引数・`CLAUDE_GATE_FILES` の root 外のパスをグラフに取り込み、unreachable-file として報告する

## depgraph の Findings — 0 件（2026-11-04 01:02 scan）

当初の warn 6 件はすべて対応済み:
- duplicate-code（tail の重複）: `pkg/internal/textutil.Tail` に共通化（テスト `tail_test.go`）
- duplicate-code（figma の `collectText` / `collectVariants`）: 走査を `walk` / `children` に共通化（先に挙動を固定するテスト `figma_test.go` を追加）
- duplicate-symbol `Example`: checker 側を `ExampleChecker` に改名（`domain.Example` と別物なので）
- duplicate-symbol `Repo`: `claimrepo.Repo` / `planfile.Repo` は Go の慣用名なので `.depgraph.yml` の `rules.duplicate-symbol.ignore` に追加
- unreachable-file（サンプル TS）: `src/app`（アプリの入口）と `src/payroll`（binding から動的 import）を `rules.unreachable-file.entries` に登録

## HANDOFF「次にやるなら」の残り

- [ ] 実プロジェクトの Markdown 仕様 1 本で `extract → lint → approve → check` を回す
- [x] 承認 UI（`spec-gate review`。2026-11-04 実装。実ブラウザでの見た目の確認は未実施）
- [ ] figma adapter を実トークンで検証
- [x] 逆変換（`spec-gate reverse`。2026-11-04 実装。LLM は `--emit` / `--collect` でサブエージェントに委譲、`SPEC_GATE_REVERSE_CMD` を設定したときだけコマンドを呼ぶ。本物の LLM でのプロンプト品質は未検証）

## docs/spec/harness.md（ハーネス拡張）— 実装済み

- [x] extract / reverse のサブエージェント委譲（`--emit` / `--collect`、`spec-gate-llm-worker`、skills）
- [x] ランダム入力による検証（`fuzz:` / `relations:`）
- [x] 見出し変更時の再リンク提案（「もしかして」）
- [x] 変更検知の精度（binding の実在パス、Go パッケージの依存）
- [x] mutation score（`plan mutate`、front matter の `mutation:` で `plan freeze` を拒否）
- [x] holdout（`out_sha256`、`holdout add`、エージェントの読み書き拒否）
- [x] リテラル漏れ検知（`spec-gate leak`、hook-gate の post_edit で警告）
- [x] CI 用の出力と指標（`check --json`、`metrics`）
- [x] 役割別レポート（`check --role`）
- [x] 曖昧の記録（承認 UI の「曖昧」、`claims/_ambiguous/`、coverage の要確認）
- [x] 通り方のログ（`.spec-gate/trace.jsonl`、`spec-gate trace`）
- [x] リモート承認（`review ask` / `review record`）
- [ ] リモート承認を実際の Claude Code セッションで確認する（hook に渡る回答の形が未検証）
- [x] README（英語・日本語）を上記の機能に合わせて更新

## 2026-11-04 再抽出の前後比較（docs/spec/harness.md の 13 節 × 3 run）

| | 前（#28） | 後（#29） |
|---|---|---|
| 候補 | 149 | 120 |
| 読めない候補 | 13 | 3 |
| ID 衝突（`-2` など） | 34 | 16 |
| 全員一致（3/3） | 89（65%） | 86（73%） |
| example の候補 / 例の行 | 23 / 60 | 14 / 44 |
| 試し実行 pass / fail / error | 13 / 1 / 9 | 14 / 0 / 0 |
| 入力が JSON 文字列の例 | 22 | 0 |
| manual の割合（読める候補のうち） | 83% | 88% |
| hint のある manual | 0 | 103 / 103 |

- 効いた: JSON 文字列の正規化と試し実行（error 0）、合議（候補・ID 衝突・読めない候補が減り、全員一致が増えた）
- 効かなかった: manual の割合。example は節の binding 表ごとに 1 件にまとまり（別々だった例が束ねられた）、残りは仕様に例の無い文章の規則。
  抽出は例を捏造しないので、manual を減らすには仕様側に例を足すか binding を足すしかない。hint はその手がかりになっている
- [ ] hint を見て、例を足せる節（mutation score の演算子一覧、fuzz の入力定義、holdout の読み込みエラーなど）に入出力例を足す
- [ ] 読めない 3 件: AMBIG-001 / AMBIG-002（kind: ui なのに story も strings も無い）、HOLDOUT-004-2（example なのに binding が無い）

## 2026-11-04 cloud セッションの環境構築

- [x] mise に gh 2.98.0 / gh-stack 0.3.0 / depgraph 0.1.2 を固定（`mise.toml`）
- [x] Neo4j: `dockerd` を起動してから `docker compose up -d`（cloud コンテナでは docker デーモンが止まっている）。depgraph scan と ARCH-001 が通る
- [x] hook-gate（feedback-gate プラグイン）: `.claude/settings.json` の `extraKnownMarketplaces` / `enabledPlugins` で有効化（次のセッションから）
- [x] 承認 UI の MCP: `.mcp.json` を `scripts/mcp.sh`（bin/spec-gate を使い回す）に変更。`go run` だと起動タイムアウトしていた
- [x] cloud セッションの準備は SessionStart hook の `scripts/session-start.sh` が行う（`CLAUDE_CODE_REMOTE=true` のときだけ。mise install、dockerd、`docker compose up -d`、bin/spec-gate のビルドをバックグラウンドで）。cloud 環境のセットアップスクリプトは他のリポジトリと共有なので使わない
- [x] depgraph の warn 4 件を解消（0 件）
  - duplicate-code（fuzz の `EvalRelation` / `evalArg`）: 式のコンパイルと評価を `runWithF` に共通化
  - duplicate-code（claimrepo の `Pending` / `Ambiguous`）: ディレクトリの読み込みを `readClaimDir`（generics）に共通化
  - duplicate-code（cli の `newExtractCmd` / `newReverseCmd`）: `--emit` / `--collect` とモード判定を `llmFlags` に共通化
  - duplicate-symbol `Prompter`: `extractor.Prompter` / `reverser.Prompter` は Repo と同じ Go の慣用名なので `.depgraph.yml` の ignore に追加
- [x] cloud セッションで spec-gate の MCP が接続タイムアウトしていた（30 秒）。session-start.sh が Neo4j イメージの pull を待ってから
      bin をビルドしていたため、MCP の起動時に bin が無く、mcp.sh が自前で（Go のダウンロード込みで）ビルドしていた。
      session-start.sh は Go だけ先に入れて bin を最初にビルドし、mcp.sh と同じロック `bin/.build.lock` を取る（mcp.sh は二重にビルドせず待つ）
- [x] それでも新しいコンテナの最初のビルドは 20〜40 秒かかり、30 秒を超えうるので、`.claude/settings.json` の `env` に
      `"MCP_TIMEOUT": "120000"` を足した（利用者の指示による）
- gh stack: このセッションの環境では GraphQL が使えないので `gh stack` のコマンドは動かない。stack の REST（`/repos/{o}/{r}/stacks`、`/pulls/{n}/merge-async`）は動く

## 承認の自動化（HOTL 承認）— 環境づくり済み（2026-11-04）

README「人間の関わり方 — HITL と HOTL」の「この先」を spec-gate 自身で開発する。

- [x] 仕様の草案 `docs/spec/auto-approval.md`（sources.yaml に登録。coverage で未カバーの節が見える）
- [x] プランの草案 `plans/2026-11-auto-approval.md`（verify 8 項目。claims は承認後に入れる）
- [x] 開発の手順のスキル `.claude/skills/spec-gate-develop/`
- [x] 人間: 仕様の未決事項 6 件を決めた（承認役は独立したエージェント、ほかはおすすめどおり）
- [x] エージェント: 節ごとに抽出（79 候補）。人間: 中心の 7 件を承認（質問ダイアログ）。プランの claims に 7 件を入れた
- [ ] 人間: plan freeze・active（スマホからはコマンドを打てないので PC で）
- [ ] 人間: 残りの候補（約 70 件、多くは manual）の承認か却下
- [x] 判定の binding 6 つ（auto-policy・auto-run-commands・auto-eligible・auto-evidence・auto-breaker・guard）を実装。承認済み 7 件は pass
- [x] `spec-gate guard`（PreToolUse hook）と承認役 `.claude/agents/spec-gate-approver.md`。settings.json に hook を追加し、今のセッションで revoke が拒否されることを確認
- [x] 実機で確認（2026-11-04、同じセッションで承認役が使えるようになった）: hook の入力に agent_type が入る。
      承認役からの auto-approve は承認役の規則で判定され（`cd … ;` を足したものは「1 つだけ」として拒否）、メインからの auto-approve は拒否された。
      仕様の「実機で確かめること」の段落は承認済み claim（APPROVER-013）の節にあるので、仕様は書き換えずここに記録する
- [x] 残りの実装（2026-11-04、利用者の指示で PdM として判断して進めた）: check での証拠の照合、`revoke` / `audit`、全経路の判定の記録、
      `spec-gate auto-approve`、metrics の auto_approved / revoked。プランの verify 9 項目はすべて通る（凍結はまだ）
  - スクラッチのプロジェクトで承認役から auto-approve を実行し、自動承認 → check pass → audit に出る → 証拠を書き換えると unapproved、を確認
  - PdM として決めたこと: 方針が無効なら auto-approve は実行しない（エラー）／逆変換は ok になりうる候補だけ（既にある ID はその前に exists）／
    exists のときは既存の証拠を上書きしないよう証拠を書かない／読めない抽出の回答は、その run が何も主張しなかったものとして数える
  - 仕様とのずれ: 仕様は「check --json の metrics」に足すと書くが、manual_rate などの指標は `spec-gate metrics` にあるので、そちらに足した
- [ ] 人間: 残りの候補の承認か却下（自動承認の仕様は約 60 件。AUDIT-001・002・004・005・009 は実装済みの規則なので承認してよい）
- [ ] 実プロジェクトで `.spec-gate/policy.yaml` を書き、本物の LLM（別モデルを並べる）で auto-approve を試す

## 自己レビュー（2026-11-05、/code-review）の対応

- [x] 判定の記録（`.spec-gate/review-decisions.jsonl`）をコミットする（.gitignore から外した）。clone ごとにブレーカーと audit が白紙に戻っていた
- [x] 既にある ID の候補は、判定にかかわらず証拠を書かない（ok 以外で既存の証拠を上書きし、正しい自動承認を unapproved にしていた）
- [x] ブレーカーの window / max_revoked が 1 未満・reset_at が RFC 3339 でない方針をエラーにする（仕様の誤りの一覧には無い。仕様に足すかは人間が判断）
- [x] reset_at を時刻として比べる。人間が承認し直した claim の差し戻しを、前の自動承認に付けない
- [x] guard の抜け道: 承認役は spec-gate のバイナリだけ（go run・環境変数の前置き・別の場所の spec-gate を拒否）。
      ほかのエージェントは $(…)・バッククォート・sh -c・eval の中身と sudo・env・timeout などの前置きの後ろも見る。go run のフラグの値を読み飛ばす
- [x] guard が動かないと素通りになる → `scripts/guard-hook.sh`（bin → go run → だめなら exit 2）
- [ ] 人間: `.claude/settings.json` の PreToolUse と `.claude/agents/spec-gate-approver.md` の hooks の command を
      `bash "$CLAUDE_PROJECT_DIR/scripts/guard-hook.sh"` に差し替える（どちらも保護済みでエージェントは書き換えられない）
- [ ] 直さないと決めた: スマホ（preview が落ちる端末）のリモート承認は、question 文で省略された例を照合できない。
      ダイアログの間エージェントは動けないので、表示から反映までに書き換える隙はほぼ無い。省略は「ほか n 件」「長いので省略」で人間に見える
- 手順のずれ（事実として記録）: 自動承認はプランを凍結する前に実装した（利用者の「PdM として判断して進めて」による）。
  guard の hook も利用者の指示で私が settings.json に入れた。凍結は人間の作業として残っている
