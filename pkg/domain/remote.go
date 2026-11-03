package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// リモート承認: Claude Code の AskUserQuestion（質問ダイアログ）で候補を見せ、
// 押された結果を PostToolUse hook で受け取って承認・却下・曖昧を反映する。
// ここには入出力の形と判定だけを置き、ファイルの移動は services が既存の承認処理で行う。

// RemoteToolName は質問ダイアログのツール名。hook の tool_name がこれ以外なら何もしない。
const RemoteToolName = "AskUserQuestion"

// RemoteHeaderMax は質問の header（ダイアログの見出し）の最大文字数。
const RemoteHeaderMax = 12

// 選択肢のラベル。
const (
	RemoteLabelApprove   = "承認"
	RemoteLabelReject    = "却下"
	RemoteLabelAmbiguous = "曖昧"
)

// RemoteAmbiguousDefaultReason は「曖昧」ボタンだけが押された（理由が書かれていない）ときの理由。
const RemoteAmbiguousDefaultReason = "リモート承認で曖昧と判断（理由未記入）"

// AskQuestion は AskUserQuestion の質問 1 つ（tool_input.questions の要素）。
type AskQuestion struct {
	Question    string      `json:"question"`
	Header      string      `json:"header"`
	MultiSelect bool        `json:"multiSelect"`
	Options     []AskOption `json:"options"`
}

// AskOption は質問の選択肢 1 つ。Preview は選択肢を選んでいる間に出る monospace の Markdown。
type AskOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
	Preview     string `json:"preview,omitempty"`
}

// RemoteVerdict はリモート承認での判定。
type RemoteVerdict string

const (
	VerdictApprove   RemoteVerdict = "approve"
	VerdictReject    RemoteVerdict = "reject"
	VerdictAmbiguous RemoteVerdict = "ambiguous"
)

// RemoteDecision は候補 1 つへの判定。Reason は曖昧のときだけ入る。
type RemoteDecision struct {
	ID      string        `json:"id"`
	Verdict RemoteVerdict `json:"verdict"`
	Reason  string        `json:"reason,omitempty"`
}

// ReviewDecision は反映した判定の記録（.spec-gate/review-decisions.jsonl の 1 行）。
type ReviewDecision struct {
	Time    string        `json:"time"`
	ID      string        `json:"id"`
	Verdict RemoteVerdict `json:"verdict"`
	Reason  string        `json:"reason,omitempty"`
	By      string        `json:"by"`
	// ContentSHA256 は人間に見せた候補の内容（preview）の sha256（16 進）
	ContentSHA256 string `json:"content_sha256"`
}

// RemoteHookInput は PostToolUse hook の stdin のうち使う部分。
type RemoteHookInput struct {
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		Questions []AskQuestion `json:"questions"`
		// Answers は読むだけで回答には使わない（エージェントが埋められるので、ここにだけ回答があれば拒否の理由を出す）
		Answers map[string]string `json:"answers,omitempty"`
	} `json:"tool_input"`
	// ToolResponse は {"answers": {...}} の形か、回答を並べた文字列
	ToolResponse json.RawMessage `json:"tool_response"`
}

var remoteQuestionRE = regexp.MustCompile(`^候補 (\S+) をどうしますか？$`)

// NewRemoteQuestion は候補 id とその preview から質問を作る。
// 3 つの選択肢すべてに同じ preview を付ける（どれを選んでいても候補の内容が見える）。
func NewRemoteQuestion(id, preview string) AskQuestion {
	header := []rune(id)
	if len(header) > RemoteHeaderMax {
		header = header[:RemoteHeaderMax]
	}
	return AskQuestion{
		Question: "候補 " + id + " をどうしますか？",
		Header:   string(header),
		Options: []AskOption{
			{Label: RemoteLabelApprove, Description: "内容は正しい。このまま仕様として確定する", Preview: preview},
			{Label: RemoteLabelReject, Description: "内容が間違っている。この候補を捨てる", Preview: preview},
			{Label: RemoteLabelAmbiguous, Description: "仕様がまだ決まっていない。保留として記録する（理由は Other に書ける）", Preview: preview},
		},
	}
}

// RemoteQuestion は承認待ち候補の質問を作る。
func RemoteQuestion(p PendingClaim) AskQuestion {
	return NewRemoteQuestion(p.Claim.ID, RenderRemotePreview(p))
}

// RemoteCandidateID は question 文から候補 ID を取り出す。形が違えば空。
func RemoteCandidateID(question string) string {
	m := remoteQuestionRE.FindStringSubmatch(question)
	if m == nil {
		return ""
	}
	return m[1]
}

var kindLabel = map[ClaimKind]string{
	KindExample:   "入出力の例で確かめる",
	KindStructure: "コードの依存関係（どこがどこを使ってよいか）で確かめる",
	KindUI:        "画面の状態と文言で確かめる",
	KindManual:    "機械では確かめられない（人が確認する）",
}

// RenderRemotePreview は候補を人間向けの Markdown にする。同じ候補なら必ず同じ文字列になる。
func RenderRemotePreview(p PendingClaim) string {
	c := p.Claim
	var b strings.Builder
	fmt.Fprintf(&b, "# %s %s\n\n", c.ID, c.Title)
	fmt.Fprintf(&b, "- 仕様の場所: %s\n", c.Source.Ref)
	fmt.Fprintf(&b, "- 種類: %s（%s）\n", kindLabel[c.Kind], c.Kind)
	if n, total, ok := strings.Cut(p.Confidence, "/"); ok {
		fmt.Fprintf(&b, "- 抽出の一致: %s 回中 %s 回で同じ内容\n", total, n)
	}
	if c.Binding != "" {
		fmt.Fprintf(&b, "- 確かめるコマンド: %s\n", c.Binding)
	}
	switch c.Kind {
	case KindExample:
		if len(c.Examples) > 0 {
			b.WriteString("\n## 例\n\n| 入力 | 期待する出力 | メモ |\n|---|---|---|\n")
			for _, ex := range c.Examples {
				fmt.Fprintf(&b, "| %s | %s | %s |\n", cell(FormatExampleInput(ex.In)), cell(FormatExampleValue(ex.Out)), cell(ex.Note))
			}
		}
		if len(c.Properties) > 0 {
			b.WriteString("\n## いつも成り立つこと\n\n")
			for _, pr := range c.Properties {
				fmt.Fprintf(&b, "- %s\n", pr)
			}
		}
	case KindStructure:
		names := make([]string, 0, len(c.Layers))
		for n := range c.Layers {
			names = append(names, n)
		}
		sort.Strings(names)
		b.WriteString("\n## 層とフォルダ\n\n| 層 | フォルダ |\n|---|---|\n")
		for _, n := range names {
			fmt.Fprintf(&b, "| %s | %s |\n", cell(n), cell(c.Layers[n]))
		}
		if c.Allowed == "from-diagram" {
			b.WriteString("\n使ってよい向きは図の矢印のとおり\n")
		}
		if len(c.Forbidden) > 0 {
			b.WriteString("\n## 使ってはいけない向き\n\n")
			for _, f := range c.Forbidden {
				fmt.Fprintf(&b, "- %s\n", f)
			}
		}
	case KindUI:
		if c.Story != "" {
			fmt.Fprintf(&b, "- 画面の部品: %s\n", c.Story)
		}
		if c.URL != "" {
			fmt.Fprintf(&b, "- URL: %s\n", c.URL)
		}
		if len(c.States) > 0 {
			fmt.Fprintf(&b, "- 状態: %s\n", strings.Join(c.States, "、"))
		}
		if len(c.Strings) > 0 {
			b.WriteString("\n## 画面に出る文言\n\n")
			for _, s := range c.Strings {
				fmt.Fprintf(&b, "- %s\n", s)
			}
		}
	}
	return b.String()
}

// cell は Markdown の表のセルに入れられる形にする（| と改行で表が崩れないように）。
func cell(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	return strings.ReplaceAll(s, "\n", " ")
}

// FormatExampleInput は例の入力を表示用にする。配列は「引数が複数」の意味なので , で並べる。
func FormatExampleInput(v any) string {
	if args, ok := v.([]any); ok {
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = FormatExampleValue(a)
		}
		return strings.Join(parts, ", ")
	}
	return FormatExampleValue(v)
}

// FormatExampleValue は値を表示用にする。文字列は数値と区別できるよう "" で囲む。
func FormatExampleValue(v any) string {
	if v == nil {
		return "なし"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

// responseAnswerRE は文字列の tool_response に並ぶ "質問"="回答" の 1 組。
var responseAnswerRE = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"="((?:[^"\\]|\\.)*)"`)

// RemoteInputAnswersRefused は回答が tool_input にだけあったときの理由。
const RemoteInputAnswersRefused = "回答が tool_response に無いため反映しない（tool_input の回答は偽造できるので使わない）"

// RemoteAnswers は hook の入力から {質問文: 回答} を取り出す。
// tool_response.answers（map）→ 文字列の tool_response の順に見る。
// tool_input.answers はエージェントが呼び出し時に埋められる欄で承認の偽造経路になるので、決して使わない。
func RemoteAnswers(in RemoteHookInput) (map[string]string, error) {
	resp := bytes.TrimSpace(in.ToolResponse)
	if len(resp) > 0 && resp[0] == '{' {
		var obj struct {
			Answers map[string]json.RawMessage `json:"answers"`
		}
		if err := json.Unmarshal(resp, &obj); err == nil && len(obj.Answers) > 0 {
			out := map[string]string{}
			for q, raw := range obj.Answers {
				out[q] = answerText(raw)
			}
			return out, nil
		}
	}
	if len(resp) > 0 && resp[0] == '"' {
		var s string
		if err := json.Unmarshal(resp, &s); err == nil {
			out := map[string]string{}
			for _, m := range responseAnswerRE.FindAllStringSubmatch(s, -1) {
				out[unescapeQuoted(m[1])] = unescapeQuoted(m[2])
			}
			if len(out) > 0 {
				return out, nil
			}
		}
	}
	if len(in.ToolInput.Answers) > 0 {
		return nil, errors.New(RemoteInputAnswersRefused)
	}
	return nil, errors.New("回答を読み取れない（tool_response.answers・tool_input.answers・回答を並べた文字列のどれでもない）")
}

// answerText は回答 1 つを文字列にする。複数選択の配列は ", " でつなぐ。
func answerText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var arr []string
	if json.Unmarshal(raw, &arr) == nil {
		return strings.Join(arr, ", ")
	}
	return string(raw)
}

func unescapeQuoted(s string) string {
	if u, err := strconv.Unquote(`"` + s + `"`); err == nil {
		return u
	}
	return s
}

// DecideRemote は hook の入力を判定のリストにする。current は候補 ID → spec-gate が今作る質問。
// 表示された質問（文・見出し・選択肢・説明・preview）が current と完全に一致するものだけを判定にする。
// 反映しなかった質問は理由を notes に入れる。
func DecideRemote(in RemoteHookInput, current map[string]AskQuestion) (decisions []RemoteDecision, notes []string) {
	decisions = []RemoteDecision{}
	if in.ToolName != RemoteToolName {
		return decisions, []string{fmt.Sprintf("tool_name が %s ではない（%q）", RemoteToolName, in.ToolName)}
	}
	answers, err := RemoteAnswers(in)
	if err != nil {
		return decisions, []string{err.Error()}
	}
	for _, q := range in.ToolInput.Questions {
		id := RemoteCandidateID(q.Question)
		if id == "" {
			notes = append(notes, fmt.Sprintf("%q は spec-gate の質問ではない", q.Question))
			continue
		}
		ans := strings.TrimSpace(answers[q.Question])
		if ans == "" {
			notes = append(notes, id+": 回答が無い")
			continue
		}
		want, ok := current[id]
		if !ok {
			notes = append(notes, id+": 承認待ちの候補に無い")
			continue
		}
		if !reflect.DeepEqual(q, want) {
			notes = append(notes, id+": 表示された内容が今の候補と一致しないので反映しない")
			continue
		}
		decisions = append(decisions, verdictFor(id, ans))
	}
	return decisions, notes
}

func verdictFor(id, answer string) RemoteDecision {
	switch answer {
	case RemoteLabelApprove:
		return RemoteDecision{ID: id, Verdict: VerdictApprove}
	case RemoteLabelReject:
		return RemoteDecision{ID: id, Verdict: VerdictReject}
	case RemoteLabelAmbiguous:
		return RemoteDecision{ID: id, Verdict: VerdictAmbiguous, Reason: RemoteAmbiguousDefaultReason}
	default:
		// Other の自由入力は、その文を理由にした曖昧
		return RemoteDecision{ID: id, Verdict: VerdictAmbiguous, Reason: answer}
	}
}
