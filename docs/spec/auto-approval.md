# 承認の自動化（HOTL 承認）仕様

本書は、claim の承認を Human-in-the-loop（人間が 1 件ずつ押す）から Human-on-the-loop（機械が条件を満たした候補を承認し、人間は見張って差し戻す）に寄せるための機能を定める。
README「人間の関わり方 — HITL と HOTL」の「この先」を実装するもの。
各節の「binding」は、その節の入出力例を検証するコマンドである（stdin に `{"in": ...}`、stdout に `{"out": ...}`）。

**状態: 草案**。「未決事項」が決まるまで抽出・承認しない。

## 背景

自己適用（`docs/spec/harness.md`）では候補が 120 件出て、承認が進んでいない。まとめ承認で「全員一致・試し実行 pass」の候補は機械が選べるようになったが、
最後のボタンは人間が押している。モデルが賢くなるほど、人間が中身を読まずに押すだけになり、承認は形だけになる。

一方で、今の証拠はエージェントが作れてしまう:

- 抽出・逆変換の LLM コマンドは環境変数（`SPEC_GATE_EXTRACT_CMD` / `SPEC_GATE_REVERSE_CMD`）で決まり、エージェントがコマンドの前に書けば差し替えられる
- 委譲（`--emit` / `--collect`）の回答（`.spec-gate-llm/`）と候補（`claims/_pending/`）は、エージェントが書ける
- `claims/_pending/` のヘッダ（`# confidence:` / `# dry-run:`）は表示と並び順にだけ使っている（承認の判断には使っていない）

このままボタンを外すと、エージェントが候補を書いて、自分で「合議で一致した」と言えてしまう。

## 原則

- **書き手と認め手を分ける**。claim を書いた AI が、その claim を正しいと認めない。認めるのは、エージェントが手を出せない経路で作った証拠と、人間が決めた方針だけ
- **証拠はエージェントが作れない経路で作る**。LLM の呼び出しは人間が決めたコマンドだけを使い、証拠は `.spec-gate/` に置く（エージェントは書けない）
- **人間は見張り、差し戻し、止められる**。自動の承認はすべて記録し、人間は一覧で見て、1 件ずつ差し戻せる。差し戻しが続けば自動承認は自分で止まる
- **ゴールは人間が決める**。仕様（`docs/spec/` など）を書くのは人間で、本書の対象外

## 承認方針

自動承認の条件は、人間が `.spec-gate/policy.yaml` に書く。エージェントは書けない（`.spec-gate/` の保護）。

```yaml
auto_approve:
  enabled: true              # 既定 false。ファイルが無いときも false
  llm_command: claude -p     # 証拠を作る LLM コマンド（stdin に prompt、stdout に応答）。必須
  runs: 3                    # 抽出の run 数（2〜10、既定 3）
  min_agree: 0               # 合意数の下限。0 は全員一致（既定 0）
  kinds: [example]           # 自動承認してよい kind（既定 [example]）
  require_reverse: true      # 逆変換の比較が一致したものだけ（既定 true）
  breaker:
    window: 20               # 直近の自動承認の件数（既定 20）
    max_revoked: 2           # その中の差し戻しがこの数に達したら止める（既定 2）
    reset_at: ""             # この時刻（RFC 3339）より前の差し戻しは数えない。空なら全部数える
```

- `policy.yaml` が読めない・`llm_command` が空・`runs` が範囲外のときは、自動承認しない（理由を出してエラー、exit 1）
- 環境変数（`SPEC_GATE_EXTRACT_CMD` など）は自動承認では使わない。エージェントが差し替えられるため

## 自動承認

`spec-gate auto-approve <ref> --area <領域>` は、仕様 1 か所について、抽出から承認までを 1 つのコマンドの中で行う。

1. `llm_command` で `runs` 回抽出し、これまでと同じ合議（「合議の同定」「manual と例のある claim の同定」）で候補を作る
2. `require_reverse` なら、各候補を `llm_command` で逆変換する（再生成 → 比較）。仕様は再生成のプロンプトに入れない
3. kind: example の候補を試し実行する
4. 各候補を「自動承認の判定」にかけ、`ok` なら `claims/<領域>/<ID>.yaml` に置いて承認する。`ok` でなければ `claims/_pending/<ID>.yaml` に置き、先頭に `# auto: <判定>` の 1 行を書く
5. 判定のたびに証拠を `.spec-gate/evidence/<ID>.json` に書く

- 途中の回答はファイルに書かない（メモリの中だけで扱う）。`.spec-gate-llm/` は使わない
- 承認は `approve` と同じ処理（lint・試し実行・仕様のハッシュ記録）で、`approved` に次を書く: `by: auto`、`at`、`source_hash`、`evidence: sha256:<証拠ファイルの sha256>`
- 同じ ID の claim が `claims/` のどこかに既にあれば、その候補は承認せず `_pending/` に置く（判定 `exists`）
- 証拠ファイルには、ref・仕様のハッシュ・`policy.yaml` の sha256・各 run の回答の sha256・候補の合意数（`n/N`）・逆変換の結果（`match` / `mismatch` / `error` / `skipped`）と食い違いの一覧・試し実行の結果・判定・時刻を書く

## 自動承認の判定

候補 1 件について、次の順に調べ、最初に当てはまったものを判定にする。どれにも当てはまらなければ `ok`。

1. `disabled`: `enabled` が false
2. `paused`: ブレーカーが止めている（「ブレーカー」）
3. `kind`: 候補の kind が `kinds` に無い
4. `agree`: 合意数が下限に届かない（`min_agree` が 0 なら全員一致でない）
5. `ambiguous`: title に「要確認」を含む
6. `dry-run`: kind: example で、試し実行が `pass` でない
7. `reverse`: `require_reverse` が true で、逆変換が `match` でない
8. `lint`: lint で拒否される

binding: `go run ./tools/bind auto-eligible`（入力は `{"policy": {"enabled", "kinds", "min_agree", "require_reverse"}, "paused": 真偽, "candidate": {"kind", "title", "confidence", "dry_run", "reverse"}}`、出力は判定。lint は binding の対象外）

| 入力 | 出力 |
|---|---|
| `{"policy": {"enabled": true, "kinds": ["example"], "min_agree": 0, "require_reverse": true}, "paused": false, "candidate": {"kind": "example", "title": "円未満は切り捨て", "confidence": "3/3", "dry_run": "pass", "reverse": "match"}}` | `"ok"` |
| `{"policy": {"enabled": false, "kinds": ["example"], "min_agree": 0, "require_reverse": true}, "paused": false, "candidate": {"kind": "example", "title": "円未満は切り捨て", "confidence": "3/3", "dry_run": "pass", "reverse": "match"}}` | `"disabled"` |
| `{"policy": {"enabled": true, "kinds": ["example"], "min_agree": 0, "require_reverse": true}, "paused": true, "candidate": {"kind": "example", "title": "円未満は切り捨て", "confidence": "3/3", "dry_run": "pass", "reverse": "match"}}` | `"paused"` |
| `{"policy": {"enabled": true, "kinds": ["example"], "min_agree": 0, "require_reverse": true}, "paused": false, "candidate": {"kind": "manual", "title": "締めは月末日", "confidence": "3/3", "dry_run": "none", "reverse": "match"}}` | `"kind"` |
| `{"policy": {"enabled": true, "kinds": ["example"], "min_agree": 0, "require_reverse": true}, "paused": false, "candidate": {"kind": "example", "title": "円未満は切り捨て", "confidence": "2/3", "dry_run": "pass", "reverse": "match"}}` | `"agree"` |
| `{"policy": {"enabled": true, "kinds": ["example"], "min_agree": 2, "require_reverse": true}, "paused": false, "candidate": {"kind": "example", "title": "円未満は切り捨て", "confidence": "2/3", "dry_run": "pass", "reverse": "match"}}` | `"ok"` |
| `{"policy": {"enabled": true, "kinds": ["example"], "min_agree": 0, "require_reverse": true}, "paused": false, "candidate": {"kind": "example", "title": "端数は切り捨て【要確認: 負の数】", "confidence": "3/3", "dry_run": "pass", "reverse": "match"}}` | `"ambiguous"` |
| `{"policy": {"enabled": true, "kinds": ["example"], "min_agree": 0, "require_reverse": true}, "paused": false, "candidate": {"kind": "example", "title": "円未満は切り捨て", "confidence": "3/3", "dry_run": "fail: 3 件中 1 件が期待と違う", "reverse": "match"}}` | `"dry-run"` |
| `{"policy": {"enabled": true, "kinds": ["example"], "min_agree": 0, "require_reverse": true}, "paused": false, "candidate": {"kind": "example", "title": "円未満は切り捨て", "confidence": "3/3", "dry_run": "pass", "reverse": "mismatch"}}` | `"reverse"` |
| `{"policy": {"enabled": true, "kinds": ["example"], "min_agree": 0, "require_reverse": false}, "paused": false, "candidate": {"kind": "example", "title": "円未満は切り捨て", "confidence": "3/3", "dry_run": "pass", "reverse": "skipped"}}` | `"ok"` |

## 証拠の照合

自動で承認した claim は、証拠と結びついているあいだだけ承認済みとみなす。

- `check` は、`approved.by` が `auto` の claim について、`.spec-gate/evidence/<ID>.json` があり、その sha256 が `approved.evidence` と一致し、証拠の判定が `ok` であることを確かめる。満たさなければ `unapproved`（detail に理由: 「自動承認の証拠が無い」「証拠が承認時と違う」「証拠の判定が ok でない」）
- 仕様が変わったときの `stale` はこれまでと同じ。自動承認した claim の再承認も、人間か `auto-approve` が行う

## 見張りと差し戻し

- 承認の経路（`approve`・承認画面・リモート承認・MCP Apps・まとめ承認・自動承認）にかかわらず、承認と却下と曖昧と差し戻しを `.spec-gate/review-decisions.jsonl` に 1 件 1 行で記録する。自動承認の行は承認者を `auto` にし、証拠ファイルのパスを加える
- `spec-gate audit [--since <時刻>]` は、自動承認した claim を新しい順に一覧する（ID・title・ref・合意数・逆変換・試し実行・時刻・証拠のパス・差し戻し済みか）。読み取りだけ。`--since` は RFC 3339 か `24h` のような期間
- `spec-gate revoke <ID> --reason <理由>` は、承認済みの claim の承認を取り消し、`claims/_pending/<ID>.yaml` に戻す（`approved` を消し、先頭に `# revoked: <理由>` の 1 行を書く）。理由は必須。記録には判定 `revoke` で書く。自動承認でない claim も取り消せる
- `check --json` の metrics に `auto_approved`（`approved.by` が `auto` の claim の数）と `revoked`（記録の中の差し戻しの数）を加える

## ブレーカー

差し戻しが続くのは、方針か証拠の作り方がずれている兆し。そのあいだは自動承認を止め、人間に戻す。

- 記録の中の自動承認を新しい順に `window` 件取り、そのうち後で差し戻されたものを数える。`reset_at` が空でなければ、その時刻より前の差し戻しは数えない
- 数が `max_revoked` 以上なら止める（判定 `paused`）。止まっているあいだも抽出と証拠の記録は行い、候補は `_pending/` に置く
- 再開は人間が `reset_at` を今の時刻にするか、方針を見直して決める（エージェントは `policy.yaml` を書けない）

binding: `go run ./tools/bind auto-breaker`（入力は `{"window", "max_revoked", "history"}`。history は自動承認を古い順に並べた `{"revoked": 真偽}` の配列（`reset_at` より前の差し戻しは false にしたもの）、出力は止めるか）

| 入力 | 出力 |
|---|---|
| `{"window": 20, "max_revoked": 2, "history": []}` | `false` |
| `{"window": 20, "max_revoked": 2, "history": [{"revoked": true}, {"revoked": false}]}` | `false` |
| `{"window": 20, "max_revoked": 2, "history": [{"revoked": true}, {"revoked": true}]}` | `true` |
| `{"window": 2, "max_revoked": 2, "history": [{"revoked": true}, {"revoked": true}, {"revoked": false}]}` | `false` |
| `{"window": 3, "max_revoked": 2, "history": [{"revoked": true}, {"revoked": true}, {"revoked": false}]}` | `true` |

## エージェントに許すこと

- エージェントは `auto-approve`・`audit` を実行してよい。承認するかどうかは方針と証拠が決め、エージェントは決められないため
- `revoke` は人間だけ。hook-gate の `pre_bash` で拒否する（`approve` と同じ規則）
- `.spec-gate/policy.yaml` と `.spec-gate/evidence/` は、これまでの `.spec-gate/` の保護でエージェントが書けない

## 未決事項

抽出の前に人間が決める。決まったら本書に書き込み、この節から消す。

1. **自動承認を走らせるのは誰か**。案: (a) エージェントが `auto-approve` を実行してよい（本書の草案）、(b) CI だけ、(c) Stop hook などハーネスだけ
2. **証拠（`.spec-gate/evidence/`）をコミットするか**。コミットしないと、別の環境の `check` で自動承認の claim が `unapproved` になる
3. **既定の kinds**。manual は check で常に unverified なので、自動承認しても守りは増えない（承認待ちの件数は減る）。example だけでよいか
4. **run の独立性**。同じモデルの run を何回重ねても、同じ思い込みは消えない。`llm_command` を run ごとに変えられるようにするか（別のモデルを混ぜる）
5. **hook-gate のルール自体の保護**。`.claude/feedback/**` と `.claude/gate.yaml` はエージェントが編集できる。ルールを消せば保護が外れるので、これも保護するか
6. **逆変換の比較の基準**。`match` だけを通すか、食い違いが「表現の違い」だけなら通すか

## 変更履歴

- 2026-11-04 草案
