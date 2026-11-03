package domain

import "context"

// ClaimRepository は claim の永続化。
type ClaimRepository interface {
	All() ([]Claim, error)
	Save(Claim) error
	// SavePending は承認待ち候補を claims/_pending に置く。
	SavePending(c Claim, confidence string) (path string, err error)
}

// PendingClaim は claims/_pending/ にある承認待ちの候補 1 件。
type PendingClaim struct {
	// Name は _pending 内のファイル名（例: ROUND-003.yaml）。候補を指す鍵として使う
	Name string
	// Path はプロジェクトルートからの相対パス
	Path string
	// Confidence は extract の合議の一致数（例: "2/3"）。ヘッダが無ければ空
	Confidence string
	Claim      Claim
	// Error は読めない・必須項目が無いときの理由。空でなければ承認できない（却下のみ）
	Error string
}

// PendingRepository は承認待ち候補（claims/_pending/）の操作。
type PendingRepository interface {
	// Pending は承認待ち候補を名前順に返す。壊れた候補も Error つきで返す。
	Pending() ([]PendingClaim, error)
	// Promote は候補 name を claims/<area>/ に移し、移動後の絶対パスを返す。
	Promote(name, area string) (path string, err error)
	// Demote は Promote で移したファイルを _pending に戻す（承認に失敗したときの巻き戻し）。
	Demote(path string) error
	// DiscardPending は候補 name を削除する（却下）。
	DiscardPending(name string) error
}

// HoldoutRepository は holdout/<claim-id>.yaml（エージェントに見せない入出力例）の読み込み。
// 期待値は out_sha256（ハッシュ）でしか持たない。平文の out があるファイルはエラー。
// 読み込みエラーにも holdout の値を含めない（エラー文はエージェントに見える）。
type HoldoutRepository interface {
	// Holdout は claim の holdout を返す。ファイルが無ければ nil, nil。
	Holdout(claimID string) ([]HoldoutExample, error)
	// All は holdout のある claim ID → holdout。lint・leak 用。
	All() (map[string][]HoldoutExample, error)
}

// HoldoutWriter は holdout への追記（spec-gate holdout add。人間だけが使う）。
type HoldoutWriter interface {
	// Append は claim の holdout の末尾に 1 件足し、書いたファイルのルート相対パスを返す。
	Append(claimID string, ex HoldoutExample) (path string, err error)
}

// SourceRegistry は sources.yaml。
type SourceRegistry interface {
	All() ([]SourceEntry, error)
}

// SourceAdapter は仕様ソース 1 種類の読み方。
type SourceAdapter interface {
	Type() SourceType
	// Snapshot は ref の現在の状態（ハッシュと抜粋）を返す。
	Snapshot(ctx context.Context, ref string) (Snapshot, error)
	// Units は coverage 用に、ref を「claim に紐づけられる単位」に分ける。
	// markdown なら見出し、mermaid ならエッジ。
	Units(ctx context.Context, ref string) ([]string, error)
}

// Checker は claim 1 種類の検証。
type Checker interface {
	Kind() ClaimKind
	Check(ctx context.Context, c Claim, opt CheckOptions) Result
}

// CheckOptions は check の実行オプション。
type CheckOptions struct {
	// UI が true のとき Playwright 等の実ブラウザ検証を行う。false なら strings は unverified。
	UI bool
	// DiagramEdges は structure の allowed: from-diagram 用に service が渡す。
	DiagramEdges map[string]bool
	// Holdout は kind: example の holdout。service が HoldoutRepository から読んで渡す。
	// checker は examples に続けて流し、結果は件数だけを出す。
	Holdout []HoldoutExample
}

// PlanRepository はプランファイルの読み込みと凍結記録。
type PlanRepository interface {
	All() ([]Plan, error)
	Load(path string) (Plan, error)
	Lock(path string) (*PlanLock, error)
	Freeze(path string, lock PlanLock) error
}

// CommandRunner は verify 等のコマンド実行。
type CommandRunner interface {
	Run(ctx context.Context, command string) (output string, err error)
}

// SourceFiles は mutation score 用に、claim が触るソースファイルを読み書きする。
type SourceFiles interface {
	// Expand は claim が触るパス（ファイル・ディレクトリ）を、変異を入れる対象のソースファイルに展開する。
	// テストファイルや実在しないパスは除き、名前順で返す。
	Expand(paths []string) ([]string, error)
	Read(path string) (string, error)
	// Write はファイルの中身を置き換える（パーミッションは保つ）。
	Write(path, content string) error
}

// ChangeDetector は作業ツリーで変更されたファイル。
type ChangeDetector interface {
	ChangedFiles() ([]string, error)
}

// Extractor は仕様テキストから claim 候補を抽出する（LLM）。
type Extractor interface {
	Extract(ctx context.Context, ref, excerpt string) ([]Claim, error)
}

// Clock は現在時刻（テスト用に差し替え可能）。
type Clock interface {
	Now() string
}

// Reverser は claim を仕様文に逆変換し、元の仕様と突き合わせる（LLM）。
type Reverser interface {
	// Restate は claim の内容だけ（元の仕様テキストは見せない）から仕様文を書き起こす。
	Restate(ctx context.Context, c Claim) (string, error)
	// Compare は再生成した仕様文と元の仕様テキストを比べる。
	Compare(ctx context.Context, restated, original string) (Comparison, error)
}

// LLMWorkspace は LLM 処理をサブエージェントに委譲するときの作業ディレクトリ（.spec-gate-llm/）。
// パスはプロジェクトルートからの相対（区切りは /）。
type LLMWorkspace interface {
	// Write は rel にファイルを書く（親ディレクトリも作る）。
	Write(rel, content string) error
	// Read は rel を読む。無ければ ok=false（error ではない）。
	Read(rel string) (content string, ok bool, err error)
	// Abs は rel の絶対パス。プロンプトに回答の書き先として書く。
	Abs(rel string) string
	// RemoveAll は rel 以下を消す。
	RemoveAll(rel string) error
}

// ExtractPrompter は抽出のプロンプトの組み立てと回答の解釈（LLM は呼ばない）。
type ExtractPrompter interface {
	// Prompt は抽出のルールと仕様の抜粋をすべて含んだプロンプトを返す。
	Prompt(ref, excerpt string) string
	// Parse は回答から claim 候補を読む。source.ref は ref にそろえる。
	Parse(answer, ref string) ([]Claim, error)
}

// ReversePrompter は逆変換のプロンプトの組み立てと回答の解釈（LLM は呼ばない）。
type ReversePrompter interface {
	// RestatePrompt は claim の内容だけ（元の仕様は含めない）から仕様文を書き起こさせるプロンプト。
	RestatePrompt(c Claim) (string, error)
	// ParseRestate は再生成の回答から仕様文を取り出す。空なら error。
	ParseRestate(answer string) (string, error)
	// ComparePrompt は再生成した仕様文と元の仕様を比べさせるプロンプト。
	ComparePrompt(restated, original string) string
	// ParseCompare は比較の回答を読む。読めなければ error（一致とはみなさない）。
	ParseCompare(answer string) (Comparison, error)
}
