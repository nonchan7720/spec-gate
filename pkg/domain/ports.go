package domain

import "context"

// ClaimRepository は claim の永続化。
type ClaimRepository interface {
	All() ([]Claim, error)
	Save(Claim) error
	// SavePending は承認待ち候補を claims/_pending に置く。
	SavePending(c Claim, confidence string) (path string, err error)
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
