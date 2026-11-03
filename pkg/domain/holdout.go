package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// holdout はエージェントに見せない入出力例（holdout/<claim-id>.yaml）。
// check では examples に続けて binding に流すが、レポートやエージェントへの出力には
// 入力・期待値・実際の値を一切出さず、件数だけを出す。
// シェルを使えるエージェントには holdout ファイルを隠し切れないので、期待値は平文で持たず
// 正規化 JSON の SHA-256（out_sha256）だけを持つ（docs/spec/harness.md「holdout examples」）。

// HoldoutExample は holdout 1 件。In は binding に流すので平文、期待値はハッシュだけ。
type HoldoutExample struct {
	In        any
	OutSHA256 string
}

// CanonicalJSON は v の正規化 JSON を返す。値を JSON として読み直し（数値は float64）、
// encoding/json で書く。オブジェクトのキーは辞書順、HTML エスケープはせず、末尾の改行は付けない。
func CanonicalJSON(v any) ([]byte, error) {
	b, err := encodeJSON(normalizeNumber(v))
	if err != nil {
		return nil, err
	}
	// 読み直して int / float64 や構造体などの違いを JSON の値に揃える
	var decoded any
	if err := json.Unmarshal(b, &decoded); err != nil {
		return nil, err
	}
	return encodeJSON(decoded)
}

func encodeJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// OutputHash は v の正規化 JSON の SHA-256（16 進小文字）。holdout の out_sha256 と比べる値。
func OutputHash(v any) (string, error) {
	b, err := CanonicalJSON(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// ValidOutputHash は s が out_sha256 として正しい形（16 進小文字 64 文字）か。
func ValidOutputHash(s string) bool {
	if len(s) != sha256.Size*2 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// HoldoutInputValues は holdout の in の値を平らにして返す（漏れ検知用。期待値は平文で無いので in だけ）。
func HoldoutInputValues(exs []HoldoutExample) []any {
	vs := make([]any, 0, len(exs))
	for _, ex := range exs {
		vs = append(vs, ex.In)
	}
	return flatten(vs)
}

// HoldoutResult は holdout 1 件の判定結果。値は持たない（持たせると出力に混ざる危険がある）。
type HoldoutResult struct {
	Pass bool `json:"pass"`
}

// HoldoutSummary はレポートに出す 1 行。件数だけを出し、値は出さない。
func HoldoutSummary(results []HoldoutResult) string {
	if len(results) == 0 {
		return "holdout なし"
	}
	failed := 0
	for _, r := range results {
		if !r.Pass {
			failed++
		}
	}
	if failed == 0 {
		return fmt.Sprintf("holdout %d 件すべて一致", len(results))
	}
	return fmt.Sprintf("holdout %d 件中 %d 件が不一致（値は表示しない）", len(results), failed)
}

// LintHoldout は holdout の形だけを確かめる（対応する claim があり kind: example であること、
// out_sha256 が正しい形であること）。期待値は平文で無いので properties との整合は見ない
// （properties は check のときに実際の出力で評価する）。メッセージには番号だけを出し、値は出さない。
// holdout は claim ID → holdout。
func LintHoldout(claims []Claim, holdout map[string][]HoldoutExample) []LintIssue {
	byID := map[string]Claim{}
	for _, c := range claims {
		byID[c.ID] = c
	}
	ids := make([]string, 0, len(holdout))
	for id := range holdout {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var issues []LintIssue
	for _, id := range ids {
		c, ok := byID[id]
		switch {
		case !ok:
			issues = append(issues, LintIssue{id, "holdout に対応する claim が無い"})
			continue
		case c.Kind != KindExample:
			issues = append(issues, LintIssue{id, fmt.Sprintf("holdout は kind: example の claim にだけ使える（kind: %s）", c.Kind)})
			continue
		}
		for i, ex := range holdout[id] {
			if !ValidOutputHash(ex.OutSHA256) {
				issues = append(issues, LintIssue{id, fmt.Sprintf("holdout[%d] の out_sha256 が SHA-256 の 16 進小文字 64 文字ではない（値は表示しない）", i)})
			}
		}
	}
	return issues
}
