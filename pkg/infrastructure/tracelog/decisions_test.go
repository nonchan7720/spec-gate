package tracelog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

func TestDecisions_AppendsOneLinePerDecision(t *testing.T) {
	root := t.TempDir()
	l := NewDecisions(root)
	for _, d := range []domain.ReviewDecision{
		{Time: "t1", ID: "A", Verdict: domain.VerdictApprove, By: "alice", ContentSHA256: "aa"},
		{Time: "t2", ID: "B", Verdict: domain.VerdictAmbiguous, Reason: "締め日が未定", By: "alice", ContentSHA256: "bb"},
	} {
		if err := l.Append(d); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(filepath.Join(root, ".spec-gate", "review-decisions.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"time":"t1","id":"A","verdict":"approve","by":"alice","content_sha256":"aa"}
{"time":"t2","id":"B","verdict":"ambiguous","reason":"締め日が未定","by":"alice","content_sha256":"bb"}`
	if strings.TrimSpace(string(b)) != want {
		t.Fatalf("1 件 1 行で追記:\n%s", b)
	}
}
