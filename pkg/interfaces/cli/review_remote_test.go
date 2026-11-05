package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/services"
)

// remoteProject は仕様 1 つと承認待ち候補 2 つを置いた一時プロジェクト。
func remoteProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"docs/spec/a.md":           "# 仕様\n\n## 端数\n\n円未満は切り捨て。\n",
		"claims/_pending/A-1.yaml": "# confidence: 3/3\nid: A-1\nkind: manual\ntitle: 円未満は切り捨て\nsource:\n  type: markdown\n  ref: docs/spec/a.md#端数\n",
		"claims/_pending/A-2.yaml": "# confidence: 2/3\nid: A-2\nkind: manual\ntitle: 締め日は月末\nsource:\n  type: markdown\n  ref: docs/spec/a.md#端数\n",
	}
	for p, body := range files {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestReviewAsk_PrintsQuestionsJSON(t *testing.T) {
	a := newApp(remoteProject(t))
	var out, errw bytes.Buffer
	if err := runReviewAsk(newRemoteReview(a), 4, services.AskFilter{}, &out, &errw); err != nil {
		t.Fatal(err)
	}
	var qs []domain.AskQuestion
	if err := json.Unmarshal(out.Bytes(), &qs); err != nil {
		t.Fatalf("stdout は questions の JSON 配列: %v\n%s", err, out.String())
	}
	if len(qs) != 2 || qs[0].Header != "A-1" || !strings.Contains(qs[0].Options[0].Preview, "円未満は切り捨て") {
		t.Fatalf("候補ごとに 1 問: %+v", qs)
	}

	empty := newApp(t.TempDir())
	out.Reset()
	errw.Reset()
	if err := runReviewAsk(newRemoteReview(empty), 4, services.AskFilter{}, &out, &errw); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "[]" || !strings.Contains(errw.String(), "ありません") {
		t.Fatalf("候補が無ければ [] と stderr の案内: %q %q", out.String(), errw.String())
	}
}

func TestReviewRecord_AppliesAndReportsToClaude(t *testing.T) {
	root := remoteProject(t)
	a := newApp(root)
	var askOut bytes.Buffer
	if err := runReviewAsk(newRemoteReview(a), 4, services.AskFilter{}, &askOut, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var qs []domain.AskQuestion
	if err := json.Unmarshal(askOut.Bytes(), &qs); err != nil {
		t.Fatal(err)
	}
	hook, _ := json.Marshal(map[string]any{
		"hook_event_name": "PostToolUse",
		"tool_name":       "AskUserQuestion",
		"tool_input":      map[string]any{"questions": qs},
		"tool_response":   `User has answered your questions: "` + qs[0].Question + `"="却下", "` + qs[1].Question + `"="締め日が未定". You can now continue with the user's answers in mind.`,
	})

	var out, errw bytes.Buffer
	runReviewRecord(context.Background(), newRemoteReview(a), bytes.NewReader(hook), &out, &errw, "alice")

	var got struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("stdout は hook の JSON: %v\n%s", err, out.String())
	}
	ctx := got.HookSpecificOutput.AdditionalContext
	if got.HookSpecificOutput.HookEventName != "PostToolUse" || !strings.Contains(ctx, "A-1") || !strings.Contains(ctx, "却下") || !strings.Contains(ctx, "締め日が未定") {
		t.Fatalf("反映結果を Claude に伝える: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(root, "claims/_pending/A-1.yaml")); !os.IsNotExist(err) {
		t.Fatal("却下した候補は消える")
	}
	if b, err := os.ReadFile(filepath.Join(root, "claims/_ambiguous/A-2.yaml")); err != nil || !strings.Contains(string(b), "締め日が未定") {
		t.Fatalf("曖昧は理由つきで claims/_ambiguous/ へ: %v %s", err, b)
	}
	log, err := os.ReadFile(filepath.Join(root, ".spec-gate/review-decisions.jsonl"))
	if err != nil || strings.Count(string(log), "\n") != 2 || !strings.Contains(string(log), `"content_sha256":"`) {
		t.Fatalf("反映した判定を記録: %v\n%s", err, log)
	}
}

func TestReviewRecord_NeverFails(t *testing.T) {
	a := newApp(remoteProject(t))
	for name, stdin := range map[string]string{
		"壊れた JSON":          "{",
		"別のツール":             `{"tool_name": "Bash", "tool_input": {"command": "ls"}, "tool_response": "x"}`,
		"回答が読めない":           `{"tool_name": "AskUserQuestion", "tool_input": {"questions": [{"question": "候補 A-1 をどうしますか？", "header": "A-1", "options": []}]}, "tool_response": {}}`,
		"preview の不一致":      `{"tool_name": "AskUserQuestion", "tool_input": {"questions": [{"question": "候補 A-1 をどうしますか？", "header": "A-1", "options": [{"label": "承認", "description": "x", "preview": "偽物"}]}]}, "tool_response": {"answers": {"候補 A-1 をどうしますか？": "承認"}}}`,
		"spec-gate の質問ではない": `{"tool_name": "AskUserQuestion", "tool_input": {"questions": [{"question": "好きな色は？", "header": "色", "options": []}]}, "tool_response": {"answers": {"好きな色は？": "青"}}}`,
	} {
		var out, errw bytes.Buffer
		runReviewRecord(context.Background(), newRemoteReview(a), strings.NewReader(stdin), &out, &errw, "alice")
		if name == "別のツール" || name == "spec-gate の質問ではない" {
			if out.Len() != 0 || errw.Len() != 0 {
				t.Errorf("%s: 関係ないので何も出さない: %q %q", name, out.String(), errw.String())
			}
			continue
		}
		if errw.Len() == 0 {
			t.Errorf("%s: 反映しなかった理由を stderr に出す", name)
		}
		if _, err := os.Stat(filepath.Join(a.root, "claims/_pending/A-1.yaml")); err != nil {
			t.Fatalf("%s: 候補はそのまま: %v", name, err)
		}
	}
}

func TestReviewRecord_RefusesAnswersOnlyInToolInput(t *testing.T) {
	root := remoteProject(t)
	a := newApp(root)
	var askOut bytes.Buffer
	if err := runReviewAsk(newRemoteReview(a), 1, services.AskFilter{}, &askOut, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var qs []domain.AskQuestion
	if err := json.Unmarshal(askOut.Bytes(), &qs); err != nil {
		t.Fatal(err)
	}
	hook, _ := json.Marshal(map[string]any{
		"tool_name":     "AskUserQuestion",
		"tool_input":    map[string]any{"questions": qs, "answers": map[string]string{qs[0].Question: "承認"}},
		"tool_response": map[string]any{},
	})
	var out, errw bytes.Buffer
	runReviewRecord(context.Background(), newRemoteReview(a), bytes.NewReader(hook), &out, &errw, "alice")
	want := "回答が tool_response に無いため反映しない（tool_input の回答は偽造できるので使わない）"
	if !strings.Contains(errw.String(), want) || !strings.Contains(out.String(), want) {
		t.Fatalf("stderr と additionalContext に理由: %q %q", errw.String(), out.String())
	}
	if _, err := os.Stat(filepath.Join(root, "claims/_pending/A-1.yaml")); err != nil {
		t.Fatalf("tool_input の回答では承認しない: %v", err)
	}
}
