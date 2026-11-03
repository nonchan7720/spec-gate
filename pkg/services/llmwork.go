package services

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// LLMWorkDir は LLM 処理をサブエージェントに委譲するときの作業ディレクトリ（プロジェクトルート相対）。
// .spec-gate/ はエージェントの書き込みを禁止しているので使わない。git の管理対象外。
const LLMWorkDir = ".spec-gate-llm"

const (
	extractWorkDir = LLMWorkDir + "/extract"
	reverseWorkDir = LLMWorkDir + "/reverse"
	manifestName   = "manifest.json"
)

// llmName は ref や claim ID を作業ディレクトリ内の名前にする（/・#・空白を _ に置き換える）。
func llmName(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '#' || unicode.IsSpace(r) {
			return '_'
		}
		return r
	}, s)
}

// extractDir は ref の抽出の作業ディレクトリ。
func extractDir(ref string) string { return extractWorkDir + "/" + llmName(ref) }

// ExtractPromptPaths は extract --emit が書く run ごとのプロンプトのパス（ルート相対）。
func ExtractPromptPaths(ref string, runs int) []string {
	out := make([]string, 0, runs)
	for k := 1; k <= runs; k++ {
		out = append(out, fmt.Sprintf("%s/run-%d.prompt.md", extractDir(ref), k))
	}
	return out
}

func extractAnswerPath(ref string, k int) string {
	return fmt.Sprintf("%s/run-%d.answer.md", extractDir(ref), k)
}

// reverse の段階。
const (
	ReversePhaseRestate = "restate" // 再生成の回答待ち
	ReversePhaseCompare = "compare" // 比較の回答待ち
	ReversePhaseDone    = "done"    // レポート済み
)

func reversePromptPath(id, phase string) string {
	return fmt.Sprintf("%s/%s.%s.prompt.md", reverseWorkDir, llmName(id), phase)
}

func reverseAnswerPath(id, phase string) string {
	return fmt.Sprintf("%s/%s.%s.answer.md", reverseWorkDir, llmName(id), phase)
}

// taskPrompt はサブエージェント 1 体に渡す自己完結したプロンプト。
// 回答の書き先（絶対パス）と、他のファイルを読まないという約束を本文の前に置く。
func taskPrompt(answerAbs, body string) string {
	return fmt.Sprintf(`# spec-gate の LLM タスク（独立した 1 件）

これは spec-gate が書き出した、独立した 1 件の判断です。同じ仕様について別の担当者が同時に独立に答えています。
合議の独立性を保つため、次を守ってください。

- 根拠にするのは、このファイルに書かれていることだけです
- このファイル以外のファイルを読んだり一覧したりしないでください（他の run / claim のプロンプトや回答、claims/、リポジトリのコードや仕様を含む）
- 回答は、次のファイル 1 つだけに書いてください。ほかのファイルは作らない・書き換えないでください

  回答ファイル: %s

- 回答ファイルには、下の「タスク」が求める出力だけを書いてください（前置き・感想・作業報告は不要）

---

## タスク

%s
`, answerAbs, strings.TrimSpace(body))
}

// MissingAnswersError は collect したときに回答がそろっていないこと。
type MissingAnswersError struct {
	// Missing は欠けている run（extract は run-<k>、reverse は <claim-id>.<段階>）
	Missing []string
	// Paths は Missing と同じ順の、書かれるべき回答ファイルの絶対パス
	Paths []string
}

func (e *MissingAnswersError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "回答が %d 件欠けている（サブエージェントが書き終えてから collect する）:", len(e.Missing))
	for i, m := range e.Missing {
		fmt.Fprintf(&b, "\n  - %s: %s", m, e.Paths[i])
	}
	return b.String()
}

// readManifest は作業ディレクトリの manifest.json を読む。無ければ ok=false。
func readManifest(work domain.LLMWorkspace, dir string, v any) (bool, error) {
	s, ok, err := work.Read(dir + "/" + manifestName)
	if err != nil || !ok {
		return false, err
	}
	if err := json.Unmarshal([]byte(s), v); err != nil {
		return false, fmt.Errorf("%s/%s が読めない: %w", dir, manifestName, err)
	}
	return true, nil
}

func writeManifest(work domain.LLMWorkspace, dir string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return work.Write(dir+"/"+manifestName, string(b)+"\n")
}
