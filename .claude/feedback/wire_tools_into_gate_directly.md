---
name: wire_tools_into_gate_directly
description: プロジェクトに導入した検出ツールは、ツールの推奨どおり品質ゲート（gate.yaml）と hook に直接組み込む。別チェック経由の間接呼び出しで済ませない
type: feedback
count: 1
---

プロジェクトに検出ツール（依存解析・lint・セキュリティスキャンなど）を導入・連携したら、そのツールの README にある推奨設定
（`.claude/gate.yaml` の rule、SessionStart / UserPromptSubmit などの hook）をそのまま組み込む。
別のチェックの内部から呼ばれているだけでは、組み込んだことにならない。

**Excuse:** 「spec-gate の structure claim の中で `depgraph gate` を呼んでいるので、gate.yaml には入れなくても動く」

**指摘:** 間接呼び出しは、その claim が対象にしたファイル（サンプルアプリの層）にしか効かず、プロジェクト本体のコードを編集しても検出器が走らない。
基準を記録する hook も無いので、セッションの差分判定も成り立たない。ツールが想定する入口（gate の rule と hook）に直接つなぐこと。

**Why:** 導入したツールが、実際の編集に対して漏れなく動くようにするため。

**How to apply:** 新しいツールを導入したり設定ファイルを置いたりしたとき。そのツールの README で「品質ゲート」「hook」の節を確認し、
gate.yaml の rule（対象ファイルの match を含む）と settings.json の hook を追加する。追加後、実際にファイルを編集して gate の出力でツールが走ったことを確かめる。
