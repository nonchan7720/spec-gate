package main

import (
	"bytes"
	"strings"
	"testing"
)

// harness.md「リモート承認」の表で使う、候補 R-1（preview は "P"）の質問。
const remoteQ = `{"question": "候補 R-1 をどうしますか？", "header": "R-1", "multiSelect": false, "options": [{"label": "承認", "description": "内容は正しい。このまま仕様として確定する", "preview": "P"}, {"label": "却下", "description": "内容が間違っている。この候補を捨てる", "preview": "P"}, {"label": "曖昧", "description": "仕様がまだ決まっていない。保留として記録する（理由は Other に書ける）", "preview": "P"}]}`

// harness.md「リモート承認」の表
func TestBinding_RemoteDecisions(t *testing.T) {
	assertRows(t, "remote-decisions", []bindRow{
		{`{"hook": {"tool_name": "AskUserQuestion", "tool_input": {"questions": [` + remoteQ + `]}, "tool_response": {"answers": {"候補 R-1 をどうしますか？": "承認"}}}, "previews": {"R-1": "P"}}`,
			`[{"id": "R-1", "verdict": "approve"}]`},
		{`{"hook": {"tool_name": "AskUserQuestion", "tool_input": {"questions": [` + remoteQ + `]}, "tool_response": "User has answered your questions: \"候補 R-1 をどうしますか？\"=\"却下\". You can now continue with the user's answers in mind."}, "previews": {"R-1": "P"}}`,
			`[{"id": "R-1", "verdict": "reject"}]`},
		{`{"hook": {"tool_name": "AskUserQuestion", "tool_input": {"questions": [` + remoteQ + `], "answers": {"候補 R-1 をどうしますか？": "曖昧"}}}, "previews": {"R-1": "P"}}`,
			`[]`},
		{`{"hook": {"tool_name": "AskUserQuestion", "tool_input": {"questions": [` + remoteQ + `]}, "tool_response": "User has answered your questions: \"候補 R-1 をどうしますか？\"=\"曖昧\". You can now continue with the user's answers in mind."}, "previews": {"R-1": "P"}}`,
			`[{"id": "R-1", "verdict": "ambiguous", "reason": "リモート承認で曖昧と判断（理由未記入）"}]`},
		{`{"hook": {"tool_name": "AskUserQuestion", "tool_input": {"questions": [` + remoteQ + `]}, "tool_response": {"answers": {"候補 R-1 をどうしますか？": "締め日の扱いが決まっていない"}}}, "previews": {"R-1": "P"}}`,
			`[{"id": "R-1", "verdict": "ambiguous", "reason": "締め日の扱いが決まっていない"}]`},
		{`{"hook": {"tool_name": "AskUserQuestion", "tool_input": {"questions": [` + remoteQ + `]}, "tool_response": {"answers": {"候補 R-1 をどうしますか？": "承認"}}}, "previews": {"R-1": "Q"}}`,
			`[]`},
		{`{"hook": {"tool_name": "AskUserQuestion", "tool_input": {"questions": [` + remoteQ + `]}, "tool_response": {"answers": {"候補 R-1 をどうしますか？": "承認"}}}, "previews": {}}`,
			`[]`},
		{`{"hook": {"tool_name": "Bash", "tool_input": {"questions": [` + remoteQ + `]}, "tool_response": {"answers": {"候補 R-1 をどうしますか？": "承認"}}}, "previews": {"R-1": "P"}}`,
			`[]`},
		{`{"hook": {"tool_name": "AskUserQuestion", "tool_input": {"questions": [` + remoteQ + `]}, "tool_response": {}}, "previews": {"R-1": "P"}}`,
			`[]`},
	})
}

func TestBinding_RemoteDecisionsRejectsNonObject(t *testing.T) {
	var out bytes.Buffer
	if err := funcs.run("remote-decisions", strings.NewReader(`{"in": "x"}`), &out); err == nil {
		t.Fatalf("オブジェクトでなければエラー: %q", out.String())
	}
}
