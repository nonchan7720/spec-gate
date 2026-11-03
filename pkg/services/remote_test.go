package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

type memDecisions struct {
	got []domain.ReviewDecision
	err error
}

func (m *memDecisions) Append(d domain.ReviewDecision) error {
	if m.err != nil {
		return m.err
	}
	m.got = append(m.got, d)
	return nil
}

func newRemote(pending []domain.PendingClaim) (*RemoteReviewService, *fakeStore, *memDecisions) {
	rv, store := newReview(pending, map[string]domain.Snapshot{"s.md#a": {Hash: "h1"}})
	store.ambiguous = map[string]string{}
	log := &memDecisions{}
	return &RemoteReviewService{Review: rv, Log: log, Clock: fixedClock("2026-11-04T00:00:00Z")}, store, log
}

func hookJSON(t *testing.T, questions []domain.AskQuestion, answers map[string]string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"tool_name":     "AskUserQuestion",
		"tool_input":    map[string]any{"questions": questions},
		"tool_response": map[string]any{"answers": answers},
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRemoteAsk(t *testing.T) {
	var ps []domain.PendingClaim
	for _, id := range []string{"A", "B", "C", "D", "E"} {
		ps = append(ps, pending(mdClaim(id, "s.md#a", domain.KindExample, nil), "3/3"))
	}
	ps = append(ps, domain.PendingClaim{Name: "0-broken.yaml", Error: "title がない"})
	s, _, _ := newRemote(ps)

	qs, notes, err := s.Ask(4)
	if err != nil {
		t.Fatal(err)
	}
	if len(qs) != 4 || qs[0].Question != "候補 A をどうしますか？" || qs[3].Header != "D" {
		t.Fatalf("候補順に最大 4 つ: %+v", qs)
	}
	if qs[0].Options[0].Preview != domain.RenderRemotePreview(ps[0]) {
		t.Fatalf("preview は候補のレンダリング: %q", qs[0].Options[0].Preview)
	}
	joined := strings.Join(notes, "\n")
	if !strings.Contains(joined, "0-broken.yaml") || !strings.Contains(joined, "残り 1 件") {
		t.Fatalf("読めない候補と残りの件数を案内: %v", notes)
	}
	for _, n := range []int{0, 5} {
		if _, _, err := s.Ask(n); err == nil {
			t.Errorf("--limit %d は 1〜4 の外なのでエラー", n)
		}
	}

	empty, _, _ := newRemote(nil)
	qs, notes, err = empty.Ask(4)
	if err != nil || qs == nil || len(qs) != 0 || len(notes) == 0 {
		t.Fatalf("候補が無ければ空配列と案内: %v %v %v", qs, notes, err)
	}
}

func TestRemoteRecord_AppliesThroughReviewService(t *testing.T) {
	a := pending(mdClaim("A", "s.md#a", domain.KindExample, nil), "3/3")
	b := pending(mdClaim("B", "s.md#a", domain.KindExample, nil), "3/3")
	c := pending(mdClaim("C", "s.md#a", domain.KindExample, nil), "3/3")
	s, store, log := newRemote([]domain.PendingClaim{a, b, c})
	qs, _, _ := s.Ask(4)

	res := s.Record(context.Background(), hookJSON(t, qs, map[string]string{
		qs[0].Question: "承認", qs[1].Question: "却下", qs[2].Question: "締め日が未定",
	}), "alice")

	if len(res.Outcomes) != 3 {
		t.Fatalf("3 件反映: %+v", res)
	}
	for _, o := range res.Outcomes {
		if o.Error != "" {
			t.Fatalf("失敗なし: %+v", o)
		}
	}
	if len(store.saved) != 1 || store.saved[0].ID != "A" || store.saved[0].Approved.By != "alice" {
		t.Fatalf("承認は approve と同じ処理: %+v", store.saved)
	}
	if _, ok := store.pending["B.yaml"]; ok {
		t.Fatal("却下は候補を消す")
	}
	if store.ambiguous["C.yaml"] != "締め日が未定" {
		t.Fatalf("Other の自由入力は理由つきの曖昧: %v", store.ambiguous)
	}
	sum := sha256.Sum256([]byte(domain.RenderRemotePreview(a)))
	if len(log.got) != 3 || log.got[0] != (domain.ReviewDecision{Time: "2026-11-04T00:00:00Z", ID: "A", Verdict: domain.VerdictApprove, By: "alice", ContentSHA256: hex.EncodeToString(sum[:])}) {
		t.Fatalf("反映した判定を記録: %+v", log.got)
	}
	if log.got[2].Reason != "締め日が未定" {
		t.Fatalf("曖昧の理由も記録: %+v", log.got[2])
	}
}

func TestRemoteRecord_RefusesTamperedQuestion(t *testing.T) {
	a := pending(mdClaim("A", "s.md#a", domain.KindExample, nil), "3/3")
	s, store, log := newRemote([]domain.PendingClaim{a})
	fake := domain.NewRemoteQuestion("A", "# A 問題のない候補です")

	res := s.Record(context.Background(), hookJSON(t, []domain.AskQuestion{fake}, map[string]string{fake.Question: "承認"}), "alice")
	if len(res.Outcomes) != 0 || len(res.Notes) == 0 || len(store.saved) != 0 || len(log.got) != 0 {
		t.Fatalf("preview が今の候補と違えば反映しない: %+v", res)
	}
	if _, ok := store.pending["A.yaml"]; !ok {
		t.Fatal("候補はそのまま")
	}
}

func TestRemoteRecord_ReportsFailures(t *testing.T) {
	existing := mdClaim("X", "s.md#a", domain.KindExample, approved("h1"))
	existing.Examples = []domain.Example{{In: 1, Out: 1}}
	conflict := mdClaim("A", "s.md#a", domain.KindExample, nil)
	conflict.Examples = []domain.Example{{In: 1, Out: 2}}
	s, store, log := newRemote([]domain.PendingClaim{pending(conflict, "3/3")})
	existing.File = "claims/spec/X.yaml"
	store.approved = []domain.Claim{existing}
	qs, _, _ := s.Ask(4)

	res := s.Record(context.Background(), hookJSON(t, qs, map[string]string{qs[0].Question: "承認"}), "alice")
	if len(res.Outcomes) != 1 || !strings.Contains(res.Outcomes[0].Error, "lint") || len(log.got) != 0 {
		t.Fatalf("承認が拒否されたら理由を返し、記録しない: %+v %+v", res, log.got)
	}

	// 壊れた JSON・記録の失敗でも panic せず理由を返す
	if res := s.Record(context.Background(), []byte("{"), "alice"); len(res.Notes) == 0 {
		t.Fatalf("読めない入力は理由つきで何もしない: %+v", res)
	}
	b := pending(mdClaim("B", "s.md#a", domain.KindExample, nil), "3/3")
	s2, _, log2 := newRemote([]domain.PendingClaim{b})
	log2.err = errors.New("read-only")
	qs, _, _ = s2.Ask(4)
	res = s2.Record(context.Background(), hookJSON(t, qs, map[string]string{qs[0].Question: "却下"}), "alice")
	if len(res.Outcomes) != 1 || res.Outcomes[0].Error != "" || !strings.Contains(strings.Join(res.Notes, "\n"), "read-only") {
		t.Fatalf("記録に失敗しても反映は成功として返し、記録の失敗を伝える: %+v", res)
	}
}

func TestRemoteRecord_DuplicateIDIsNotApplied(t *testing.T) {
	a1 := pending(mdClaim("A", "s.md#a", domain.KindExample, nil), "3/3")
	a2 := a1
	a2.Name = "A-2.yaml"
	s, store, _ := newRemote([]domain.PendingClaim{a1, a2})
	q := domain.RemoteQuestion(a1)
	res := s.Record(context.Background(), hookJSON(t, []domain.AskQuestion{q}, map[string]string{q.Question: "却下"}), "alice")
	if len(res.Outcomes) != 0 || len(store.pending) != 2 {
		t.Fatalf("同じ ID の候補が複数あればどれか決められないので反映しない: %+v", res)
	}
}

func TestRemoteRecord_IgnoresUnrelatedQuestions(t *testing.T) {
	s, _, _ := newRemote([]domain.PendingClaim{pending(mdClaim("A", "s.md#a", domain.KindExample, nil), "3/3")})
	q := domain.AskQuestion{Question: "好きな色は？", Header: "色", Options: []domain.AskOption{{Label: "青"}, {Label: "赤"}}}
	res := s.Record(context.Background(), hookJSON(t, []domain.AskQuestion{q}, map[string]string{q.Question: "青"}), "alice")
	if len(res.Outcomes) != 0 || len(res.Notes) != 0 {
		t.Fatalf("spec-gate の質問が 1 つも無い AskUserQuestion は黙って何もしない: %+v", res)
	}
}
