package domain

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func remoteExampleClaim() PendingClaim {
	return PendingClaim{
		Name: "ROUND-003.yaml", Confidence: "2/3",
		Claim: Claim{
			ID: "ROUND-003", Kind: KindExample, Title: "円未満は切り捨て",
			Source:     Source{Type: SourceMarkdown, Ref: "docs/spec/rounding.md#端数処理"},
			Binding:    "go run ./tools/bind round",
			Examples:   []Example{{In: 1.5, Out: 1, Note: "切り捨て"}, {In: []any{"a|b", 2}, Out: nil}},
			Properties: []string{"output <= input"},
		},
	}
}

func TestRenderRemotePreview_Example(t *testing.T) {
	got := RenderRemotePreview(remoteExampleClaim())
	for _, want := range []string{
		"ROUND-003", "円未満は切り捨て", "docs/spec/rounding.md#端数処理",
		"入出力の例", "example", "go run ./tools/bind round", "3 回中 2 回",
		"| 1.5 | 1 | 切り捨て |",
		`| "a\|b", 2 | なし |`, // 表の区切りと紛れないよう | はエスケープ
		"output <= input",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("preview に %q が無い:\n%s", want, got)
		}
	}
	if got != RenderRemotePreview(remoteExampleClaim()) {
		t.Fatal("同じ候補なら同じ preview（記録時の一致判定に使う）")
	}
}

func TestRenderRemotePreview_OtherKinds(t *testing.T) {
	st := PendingClaim{Name: "A.yaml", Claim: Claim{ID: "ARCH-002", Kind: KindStructure, Title: "層", Source: Source{Type: SourceMermaid, Ref: "docs/arch/layers.mmd"},
		Layers: map[string]string{"web": "src/web", "domain": "src/domain"}, Allowed: "from-diagram", Forbidden: []string{"domain -> web"}}}
	ui := PendingClaim{Name: "U.yaml", Claim: Claim{ID: "UI-002", Kind: KindUI, Title: "一覧", Source: Source{Type: SourceFigma, Ref: "figma://f/1"},
		Story: "src/ui/List.stories.tsx", States: []string{"空"}, Strings: []string{"データがありません"}}}
	man := PendingClaim{Name: "M.yaml", Claim: Claim{ID: "MAN-001", Kind: KindManual, Title: "手で確認", Source: Source{Type: SourceMarkdown, Ref: "a.md#x"}}}
	cases := map[string][]string{
		RenderRemotePreview(st):  {"structure", "domain", "src/domain", "web", "図", "domain -> web"},
		RenderRemotePreview(ui):  {"ui", "src/ui/List.stories.tsx", "空", "データがありません"},
		RenderRemotePreview(man): {"manual", "人"},
	}
	for got, wants := range cases {
		for _, w := range wants {
			if !strings.Contains(got, w) {
				t.Errorf("preview に %q が無い:\n%s", w, got)
			}
		}
	}
	if strings.Index(RenderRemotePreview(st), "domain") > strings.Index(RenderRemotePreview(st), "web") {
		t.Fatal("層は名前順（map の順序で preview が変わらない）")
	}
}

func TestNewRemoteQuestion(t *testing.T) {
	q := NewRemoteQuestion("VERY-LONG-ID-0001", "P")
	if q.Question != "候補 VERY-LONG-ID-0001 をどうしますか？\n\nP" || q.Header != "VERY-LONG-ID" || q.MultiSelect {
		t.Fatalf("question に候補 ID と内容（スマホでは preview が出ない）、header は 12 文字まで: %+v", q)
	}
	var labels []string
	for _, o := range q.Options {
		labels = append(labels, o.Label)
		if o.Preview != "P" || o.Description == "" {
			t.Fatalf("各選択肢に preview と説明: %+v", o)
		}
	}
	if !reflect.DeepEqual(labels, []string{"承認", "却下", "曖昧"}) {
		t.Fatalf("選択肢: %v", labels)
	}
	if id := RemoteCandidateID(q.Question); id != "VERY-LONG-ID-0001" {
		t.Fatalf("question から候補 ID を取れる: %q", id)
	}
	if id := RemoteCandidateID("好きな色は？"); id != "" {
		t.Fatalf("形が違えば空: %q", id)
	}
	b, _ := json.Marshal(q)
	if !strings.Contains(string(b), `"multiSelect":false`) || !strings.Contains(string(b), `"preview":"P"`) {
		t.Fatalf("AskUserQuestion の形の JSON: %s", b)
	}
}

func remoteHook(t *testing.T, questions []AskQuestion, response any, inputAnswers map[string]string) RemoteHookInput {
	t.Helper()
	in := RemoteHookInput{ToolName: "AskUserQuestion"}
	in.ToolInput.Questions = questions
	in.ToolInput.Answers = inputAnswers
	if response != nil {
		b, err := json.Marshal(response)
		if err != nil {
			t.Fatal(err)
		}
		in.ToolResponse = b
	}
	return in
}

func TestRemoteAnswers_Shapes(t *testing.T) {
	q1, q2 := "候補 A をどうしますか？", "候補 B をどうしますか？"
	cases := map[string]struct {
		in   RemoteHookInput
		want map[string]string
	}{
		"tool_response.answers の map": {remoteHook(t, nil, map[string]any{"answers": map[string]any{q1: "承認", q2: []any{"却下"}}}, nil), map[string]string{q1: "承認", q2: "却下"}},
		"tool_response が文字列": {remoteHook(t, nil, `User has answered your questions: "`+q1+`"="承認", "`+q2+`"="締め日が \"未定\"". You can now continue with the user's answers in mind.`, nil),
			map[string]string{q1: "承認", q2: `締め日が "未定"`}},
		"tool_input.answers は無視": {remoteHook(t, nil, map[string]any{"answers": map[string]any{q1: "却下"}}, map[string]string{q1: "承認"}), map[string]string{q1: "却下"}},
	}
	for name, tc := range cases {
		got, err := RemoteAnswers(tc.in)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: %v, want %v", name, got, tc.want)
		}
	}
	if _, err := RemoteAnswers(remoteHook(t, nil, map[string]any{"x": 1}, nil)); err == nil {
		t.Fatal("どの形でも読めなければエラー（理由を stderr に出すため）")
	}
	// tool_input はエージェントが呼び出し時に埋められる欄なので、回答がそこにだけあっても使わない
	_, err := RemoteAnswers(remoteHook(t, nil, map[string]any{"ok": true}, map[string]string{q1: "承認"}))
	if err == nil || err.Error() != RemoteInputAnswersRefused {
		t.Fatalf("tool_input にだけ回答があれば理由つきで拒否: %v", err)
	}
	_, notes := DecideRemote(remoteHook(t, []AskQuestion{NewRemoteQuestion("A", "P")}, nil, map[string]string{"候補 A をどうしますか？": "承認"}), map[string]AskQuestion{"A": NewRemoteQuestion("A", "P")})
	if len(notes) != 1 || notes[0] != "回答が tool_response に無いため反映しない（tool_input の回答は偽造できるので使わない）" {
		t.Fatalf("理由をそのまま返す: %v", notes)
	}
}

func TestDecideRemote(t *testing.T) {
	qa := NewRemoteQuestion("A", "PA")
	qb := NewRemoteQuestion("B", "PB")
	qc := NewRemoteQuestion("C", "PC")
	current := map[string]AskQuestion{"A": qa, "B": qb, "C": qc}
	answers := map[string]any{qa.Question: "承認", qb.Question: "締め日が未定", qc.Question: "曖昧"}

	got, notes := DecideRemote(remoteHook(t, []AskQuestion{qa, qb, qc}, map[string]any{"answers": answers}, nil), current)
	want := []RemoteDecision{
		{ID: "A", Verdict: VerdictApprove},
		{ID: "B", Verdict: VerdictAmbiguous, Reason: "締め日が未定"},
		{ID: "C", Verdict: VerdictAmbiguous, Reason: RemoteAmbiguousDefaultReason},
	}
	if !reflect.DeepEqual(got, want) || len(notes) != 0 {
		t.Fatalf("got %+v notes %v", got, notes)
	}

	// すり替え: 説明文を変えた質問、preview を変えた質問、今は無い候補 → どれも反映しない
	tampered := qa
	tampered.Options = append([]AskOption(nil), qa.Options...)
	tampered.Options[0].Description = "却下します"
	swapped := NewRemoteQuestion("B", "差し替えた内容")
	missing := NewRemoteQuestion("Z", "PZ")
	got, notes = DecideRemote(remoteHook(t, []AskQuestion{tampered, swapped, missing},
		map[string]any{"answers": map[string]any{qa.Question: "承認", qb.Question: "承認", missing.Question: "承認"}}, nil), current)
	if len(got) != 0 || len(notes) != 3 {
		t.Fatalf("一致しないものは反映しない: %+v %v", got, notes)
	}

	other := remoteHook(t, []AskQuestion{qa}, map[string]any{"answers": map[string]any{qa.Question: "承認"}}, nil)
	other.ToolName = "Bash"
	if got, _ := DecideRemote(other, current); len(got) != 0 {
		t.Fatalf("AskUserQuestion 以外は何もしない: %+v", got)
	}
	if got, notes := DecideRemote(remoteHook(t, []AskQuestion{qa}, map[string]any{}, nil), current); len(got) != 0 || len(notes) == 0 {
		t.Fatalf("回答が読めなければ何もせず理由を返す: %+v %v", got, notes)
	}
	if got, _ := DecideRemote(remoteHook(t, []AskQuestion{qa}, map[string]any{"answers": map[string]any{qa.Question: "  "}}, nil), current); len(got) != 0 {
		t.Fatalf("空の回答は反映しない: %+v", got)
	}
}

func TestRemoteQuestionBody(t *testing.T) {
	preview := "# R-1 円未満は切り捨て\n\n- 仕様の場所: a.md#x\n- 試し実行: pass\n\n## 例\n\n| 入力 | 期待する出力 | メモ |\n|---|---|---|\n| 1.5 | 1 |  |\n| \"a\\|b\" | 2 | 境界 |\n\n## いつも成り立つこと\n\n- isInteger(output)\n"
	want := "R-1 円未満は切り捨て\n\n- 仕様の場所: a.md#x\n- 試し実行: pass\n\n【例】\n\n・1.5 → 1\n・”a|b” → 2（境界）\n\n【いつも成り立つこと】\n\n- isInteger(output)"
	if got := RemoteQuestionBody(preview); got != want {
		t.Fatalf("見出しの # を外し、表を行にし、\" を全角にする:\n%s\n---\n%s", got, want)
	}
	var b strings.Builder
	b.WriteString("## 例\n\n| 入力 | 期待する出力 | メモ |\n|---|---|---|\n")
	for i := range 9 {
		fmt.Fprintf(&b, "| %d | %d |  |\n", i, i)
	}
	got := RemoteQuestionBody(b.String())
	if !strings.Contains(got, "・5 → 5") || strings.Contains(got, "・6 → 6") || !strings.Contains(got, "・ほか 3 件") {
		t.Fatalf("例は 6 件までで残りは件数だけ: %s", got)
	}
	long := RemoteQuestionBody("# " + strings.Repeat("あ", 200))
	if r := []rune(long); len(r) != remoteBodyLineMax+1 {
		t.Fatalf("長い行は省略する: %d", len(r))
	}
	if id := RemoteCandidateID("候補 R-1 をどうしますか？\n\n本文"); id != "R-1" {
		t.Fatalf("本文つきの question からも ID を取れる: %q", id)
	}
}
