package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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

var remoteQuestionRE = regexp.MustCompile(`^候補 (\S+) をどうしますか？(?:\n|$)`)

// NewRemoteQuestion は候補 id とその preview から質問を作る。
// preview はスマホのダイアログに表示されない（2026-11-04、Android で確認）ので、question 文にも
// preview を平文にした内容（RemoteQuestionBody）を入れる。3 つの選択肢すべてに同じ preview を付ける。
func NewRemoteQuestion(id, preview string) AskQuestion {
	header := []rune(id)
	if len(header) > RemoteHeaderMax {
		header = header[:RemoteHeaderMax]
	}
	return AskQuestion{
		Question: "候補 " + id + " をどうしますか？" + bodySuffix(RemoteQuestionBody(preview)),
		Header:   string(header),
		Options: []AskOption{
			{Label: RemoteLabelApprove, Description: "内容は正しい。このまま仕様として確定する", Preview: preview},
			{Label: RemoteLabelReject, Description: "内容が間違っている。この候補を捨てる", Preview: preview},
			{Label: RemoteLabelAmbiguous, Description: "仕様がまだ決まっていない。保留として記録する（理由は Other に書ける）", Preview: preview},
		},
	}
}

func bodySuffix(body string) string {
	if body == "" {
		return ""
	}
	return "\n\n" + body
}

// question 文に入れる内容の上限（1 行・全体の文字数、例の行数）。
const (
	remoteBodyLineMax    = 140
	remoteBodyMax        = 1500
	remoteBodyExampleMax = 6
)

// RemoteQuestionBody は preview（Markdown）を、ダイアログの question 文に入れる平文にする。
// 見出しの # を外し、例の表は「・<入力> → <出力>」の行にする。長い行・多すぎる例は省略する。
// 回答が文字列（"質問文"="回答" の並び）で返っても読めるよう、" と \ は全角（” ＼）に置き換える。
func RemoteQuestionBody(preview string) string {
	var out []string
	inTable, header, rows, omitted := false, false, 0, 0
	for line := range strings.SplitSeq(strings.TrimSpace(preview), "\n") {
		t := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(t, "|"):
			if !inTable {
				inTable, header = true, true
				continue // 表の見出し行
			}
			if strings.HasPrefix(t, "|---") || strings.HasPrefix(t, "|:") {
				continue
			}
			cells := splitCells(t)
			if header && len(cells) >= 2 {
				rows++
				if rows > remoteBodyExampleMax {
					omitted++
					continue
				}
				l := "・" + cells[0] + " → " + cells[1]
				if len(cells) > 2 && cells[2] != "" {
					l += "（" + cells[2] + "）"
				}
				out = append(out, l)
				continue
			}
			out = append(out, "・"+strings.Join(cells, " / "))
			continue
		case inTable:
			if omitted > 0 {
				out = append(out, fmt.Sprintf("・ほか %d 件", omitted))
			}
			inTable, header, rows, omitted = false, false, 0, 0
		}
		switch {
		case strings.HasPrefix(t, "## "):
			out = append(out, "【"+strings.TrimPrefix(t, "## ")+"】")
		case strings.HasPrefix(t, "# "):
			out = append(out, strings.TrimPrefix(t, "# "))
		case t == "" && len(out) > 0 && out[len(out)-1] == "":
		default:
			out = append(out, t)
		}
	}
	if inTable && omitted > 0 {
		out = append(out, fmt.Sprintf("・ほか %d 件", omitted))
	}
	for i, l := range out {
		if r := []rune(l); len(r) > remoteBodyLineMax {
			out[i] = string(r[:remoteBodyLineMax]) + "…"
		}
	}
	body := strings.TrimSpace(strings.Join(out, "\n"))
	if r := []rune(body); len(r) > remoteBodyMax {
		body = string(r[:remoteBodyMax]) + "…（長いので省略。全文は承認画面で確認する）"
	}
	return strings.NewReplacer(`"`, "”", `\`, "＼").Replace(body)
}

// splitCells は Markdown の表の行をセルに分ける（\| はセルの区切りにしない）。
func splitCells(row string) []string {
	row = strings.TrimSuffix(strings.TrimPrefix(row, "|"), "|")
	var cells []string
	var cur strings.Builder
	for i := 0; i < len(row); i++ {
		switch {
		case row[i] == '\\' && i+1 < len(row) && row[i+1] == '|':
			cur.WriteByte('|')
			i++
		case row[i] == '|':
			cells = append(cells, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteByte(row[i])
		}
	}
	return append(cells, strings.TrimSpace(cur.String()))
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
	if p.DryRun != "" {
		fmt.Fprintf(&b, "- 試し実行: %s\n", p.DryRun)
	}
	if c.Hint != "" {
		fmt.Fprintf(&b, "- 機械で確かめるには: %s\n", c.Hint)
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
	// 人が読むので、< > & を \u003c などにしない（json.Marshal の既定は HTML 向けにエスケープする）
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return fmt.Sprint(v)
	}
	return strings.TrimSuffix(buf.String(), "\n")
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
		if !sameShown(q, want) {
			notes = append(notes, id+": 表示された内容が今の候補と一致しないので反映しない")
			continue
		}
		decisions = append(decisions, verdictFor(id, ans))
	}
	return decisions, notes
}

// sameShown は、表示された質問 shown が spec-gate が今作る質問 want と同じかを確かめる。
// 質問文・見出し・選択肢のラベルと説明は完全一致を求める。preview は、shown の選択肢のどれかに preview があれば完全一致を求め、
// どれにも無ければ比べない（preview を表示しない端末では、回答の質問から preview と multiSelect が落ちる。2026-11-04 実機）。
// 候補の内容は質問文にも入れてあるので、質問文の一致で、表示された内容が今の候補と同じことを確かめられる。
func sameShown(shown, want AskQuestion) bool {
	if shown.Question != want.Question || shown.Header != want.Header || len(shown.Options) != len(want.Options) {
		return false
	}
	hasPreview := false
	for _, o := range shown.Options {
		hasPreview = hasPreview || o.Preview != ""
	}
	if hasPreview && shown.MultiSelect != want.MultiSelect {
		return false
	}
	for i, o := range shown.Options {
		w := want.Options[i]
		if o.Label != w.Label || o.Description != w.Description || (hasPreview && o.Preview != w.Preview) {
			return false
		}
	}
	return true
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
