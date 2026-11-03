package domain

// Comparison は「claim から再生成した仕様文」と「元の仕様テキスト」を突き合わせた結果。
type Comparison struct {
	// Match は両者が同じ要求を述べているとき true。
	Match bool
	// Differences は食い違い。claim にだけある主張、仕様にだけある主張、矛盾。
	Differences []string
}

// ReverseStatus は逆変換チェック 1 件の判定。
type ReverseStatus string

const (
	ReverseMatch    ReverseStatus = "match"    // 再生成した仕様文が元の仕様と一致
	ReverseMismatch ReverseStatus = "mismatch" // 食い違いがある
	ReverseSkipped  ReverseStatus = "skipped"  // 仕様テキストが読めない / テキストを持たない source
	ReverseError    ReverseStatus = "error"    // LLM の呼び出しや応答の解釈に失敗。一致とはみなさない
)

// ReverseResult は claim 1 件の逆変換チェック結果。
type ReverseResult struct {
	ID          string
	Title       string
	Status      ReverseStatus
	Restated    string   // claim だけから再生成した仕様文
	Differences []string // mismatch のときの食い違い
	Detail      string   // skipped / error の理由
	Spec        string   // 仕様の該当箇所 file:line
}
