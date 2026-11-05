package services

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// fakeLLM は prompt の種類ごとに決まった回答を返し、使ったコマンドを記録する。
type fakeLLM struct {
	commands []string
	mismatch map[string]bool // 逆変換の比較で食い違いにする claim ID
}

func (f *fakeLLM) Run(_ context.Context, command, prompt string) (string, error) {
	f.commands = append(f.commands, command)
	switch {
	case prompt == "EXTRACT":
		return "answer-" + command, nil
	case strings.HasPrefix(prompt, "RESTATE "):
		return "restated " + strings.TrimPrefix(prompt, "RESTATE "), nil
	case strings.HasPrefix(prompt, "COMPARE restated "):
		if f.mismatch[strings.TrimPrefix(prompt, "COMPARE restated ")] {
			return "mismatch", nil
		}
		return "match", nil
	}
	return "", errors.New("unexpected prompt: " + prompt)
}

// fakeAutoPrompts は抽出と逆変換のプロンプト。抽出の回答は run ごとに同じ claim を返す（全員一致）。
type fakeAutoPrompts struct{ claims []domain.Claim }

func (f fakeAutoPrompts) Prompt(string, string) string { return "EXTRACT" }
func (f fakeAutoPrompts) Parse(string, string) ([]domain.Claim, error) {
	return append([]domain.Claim(nil), f.claims...), nil
}
func (fakeAutoPrompts) RestatePrompt(c domain.Claim) (string, error) { return "RESTATE " + c.ID, nil }
func (fakeAutoPrompts) ParseRestate(a string) (string, error)        { return a, nil }
func (fakeAutoPrompts) ComparePrompt(restated, _ string) string      { return "COMPARE " + restated }
func (fakeAutoPrompts) ParseCompare(a string) (domain.Comparison, error) {
	return domain.Comparison{Match: a == "match", Differences: map[bool][]string{false: {"食い違い"}}[a == "match"]}, nil
}

type fakePolicy struct {
	f   *domain.AutoPolicyFile
	raw []byte
}

func (p fakePolicy) Load() (*domain.AutoPolicyFile, []byte, error) { return p.f, p.raw, nil }

func policyOf(t *testing.T, js string) fakePolicy {
	t.Helper()
	var f domain.AutoPolicyFile
	if err := json.Unmarshal([]byte(js), &f); err != nil {
		t.Fatal(err)
	}
	return fakePolicy{f: &f, raw: []byte(js)}
}

type memPlacer struct {
	approved map[string]domain.Claim
	pending  map[string]string // ID → 判定
}

func (m *memPlacer) PlaceApproved(c domain.Claim, area string) (string, error) {
	m.approved[c.ID] = c
	return "claims/" + area + "/" + c.ID + ".yaml", nil
}
func (m *memPlacer) SavePendingAuto(c domain.Claim, _, _, verdict string) (string, error) {
	m.pending[c.ID] = verdict
	return "claims/_pending/" + c.ID + ".yaml", nil
}

type memEvidence map[string][]byte

func (m memEvidence) Load(id string) ([]byte, bool, error) { b, ok := m[id]; return b, ok, nil }
func (m memEvidence) Save(e domain.Evidence) (string, error) {
	b, _ := json.Marshal(e)
	m[e.ID] = b
	return domain.SHA256Ref(b), nil
}

func newAuto(t *testing.T, policy fakePolicy, claims []domain.Claim, existing []domain.Claim) (*AutoApproveService, *fakeLLM, *memPlacer, memEvidence, *memDecisions) {
	t.Helper()
	llm := &fakeLLM{mismatch: map[string]bool{}}
	placer := &memPlacer{approved: map[string]domain.Claim{}, pending: map[string]string{}}
	ev, log := memEvidence{}, &memDecisions{}
	s := &AutoApproveService{
		Policy:   policy,
		Adapters: map[domain.SourceType]domain.SourceAdapter{domain.SourceMarkdown: fakeAdapter{typ: domain.SourceMarkdown, snaps: map[string]domain.Snapshot{"s.md#a": {Hash: "h1", Excerpt: "仕様"}}}},
		LLM:      llm, Extract: fakeAutoPrompts{claims: claims}, Reverse: fakeAutoPrompts{},
		Probe:  fakeProbe{"EX": {Total: 1}, "EXFAIL": {Total: 1, Mismatch: 1}},
		Claims: &fakeClaims{claims: existing}, Placer: placer, Evidence: ev, Decisions: log, Log: log, Clock: fixedClock("2026-11-04T10:00:00Z"),
	}
	return s, llm, placer, ev, log
}

func TestAutoApprove(t *testing.T) {
	policy := policyOf(t, `{"auto_approve": {"enabled": true, "llm_commands": ["model-a", "model-b"], "runs": 3}}`)
	ex := mdClaim("EX", "s.md#a", domain.KindExample, nil)
	exFail := mdClaim("EXFAIL", "s.md#a", domain.KindExample, nil)
	exFail.Title, exFail.Examples = "別の主張", []domain.Example{{In: 4, Out: 4}}
	exMis := mdClaim("EXMIS", "s.md#a", domain.KindExample, nil)
	exMis.Title, exMis.Examples = "三つ目の主張", []domain.Example{{In: 2, Out: 2}}
	manual := domain.Claim{ID: "MAN", Kind: domain.KindManual, Title: "人が確かめる規則", Source: domain.Source{Type: domain.SourceMarkdown, Ref: "s.md#a"}}
	dup := mdClaim("DUP", "s.md#a", domain.KindExample, nil)
	dup.Title, dup.Examples = "既にある主張", []domain.Example{{In: 3, Out: 3}}
	// EXMIS と DUP の試し実行は pass にする
	s, llm, placer, ev, log := newAuto(t, policy, []domain.Claim{ex, exFail, exMis, manual, dup}, []domain.Claim{mdClaim("DUP", "s.md#a", domain.KindExample, approved("h1"))})
	s.Probe = fakeProbe{"EX": {Total: 1}, "EXFAIL": {Total: 1, Mismatch: 1}, "EXMIS": {Total: 1}, "DUP": {Total: 1}}
	llm.mismatch["EXMIS"] = true

	rep, err := s.Run(context.Background(), domain.SourceMarkdown, "s.md#a", "spec")
	if err != nil {
		t.Fatal(err)
	}
	verdicts := map[string]string{}
	for _, o := range rep.Outcomes {
		verdicts[o.ID] = o.Verdict
	}
	want := map[string]string{"EX": domain.AutoOK, "EXFAIL": domain.AutoDryRun, "EXMIS": domain.AutoReverse, "MAN": domain.AutoKind, "DUP": domain.AutoExists}
	for id, v := range want {
		if verdicts[id] != v {
			t.Errorf("%s = %q, want %q", id, verdicts[id], v)
		}
	}

	// ok は承認済みで置き、証拠と結びつけ、auto として記録する
	got, ok := placer.approved["EX"]
	if !ok || got.Approved == nil || got.Approved.By != domain.AutoApproverName || got.Approved.SourceHash != "h1" || got.Approved.Evidence != domain.SHA256Ref(ev["EX"]) {
		t.Fatalf("ok の候補は証拠の sha256 つきで承認して置く: %+v", got.Approved)
	}
	if len(log.got) != 1 || log.got[0].ID != "EX" || log.got[0].By != domain.AutoApproverName || log.got[0].Evidence != domain.EvidenceRelPath("EX") {
		t.Fatalf("自動承認を記録する: %+v", log.got)
	}
	var e domain.Evidence
	_ = json.Unmarshal(ev["EX"], &e)
	if e.Verdict != domain.AutoOK || e.Confidence != "3/3" || e.Reverse != "match" || e.DryRun != "pass" || len(e.Runs) != 3 || e.PolicySHA256 != domain.SHA256Ref(policy.raw) {
		t.Fatalf("証拠に判定・合意数・逆変換・試し実行・run・方針の sha256 を書く: %+v", e)
	}
	// ok でないものは _pending に判定つきで置き、証拠も書く（exists は既にある claim の証拠を上書きしないので書かない）
	for _, id := range []string{"EXFAIL", "EXMIS", "MAN"} {
		if placer.pending[id] != want[id] || ev[id] == nil {
			t.Errorf("%s は _pending に判定つきで置き、証拠を書く: %q %v", id, placer.pending[id], ev[id] != nil)
		}
	}
	if _, wrote := ev["DUP"]; wrote || placer.pending["DUP"] != domain.AutoExists {
		t.Errorf("exists は _pending に置き、証拠は書かない")
	}
	_ = json.Unmarshal(ev["EXMIS"], &e)
	if len(e.Differences) == 0 {
		t.Errorf("逆変換の食い違いを証拠に残す: %+v", e)
	}
	// 逆変換は ok になりうる候補（EX と EXMIS）にだけ、先頭のコマンドで行う
	restates := 0
	for _, c := range llm.commands[3:] {
		if c != "model-a" {
			t.Errorf("逆変換は先頭のコマンド: %s", c)
		}
		restates++
	}
	if restates != 4 {
		t.Errorf("逆変換（再生成と比較）は 2 候補 × 2 回: %d", restates)
	}
}

func TestAutoApproveIgnoresEnvCommand(t *testing.T) {
	t.Setenv("SPEC_GATE_EXTRACT_CMD", "evil")
	t.Setenv("SPEC_GATE_REVERSE_CMD", "evil")
	policy := policyOf(t, `{"auto_approve": {"enabled": true, "llm_commands": ["model-a", "model-b"], "runs": 3}}`)
	s, llm, _, _, _ := newAuto(t, policy, []domain.Claim{mdClaim("EX", "s.md#a", domain.KindExample, nil)}, nil)
	if _, err := s.Run(context.Background(), domain.SourceMarkdown, "s.md#a", "spec"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(llm.commands[:3], ","); got != "model-a,model-b,model-a" {
		t.Fatalf("抽出は方針のコマンドを run ごとに巡回して使う: %s", got)
	}
	for _, c := range llm.commands {
		if c == "evil" {
			t.Fatal("環境変数の LLM コマンドは使わない")
		}
	}
}

func TestAutoApprove_DisabledAndPaused(t *testing.T) {
	s, _, _, _, _ := newAuto(t, fakePolicy{}, nil, nil)
	if _, err := s.Run(context.Background(), domain.SourceMarkdown, "s.md#a", "spec"); !errors.Is(err, ErrAutoDisabled) {
		t.Fatalf("方針が無ければ実行しない: %v", err)
	}
	bad := policyOf(t, `{"auto_approve": {"enabled": true, "llm_commands": []}}`)
	s, _, _, _, _ = newAuto(t, bad, nil, nil)
	if _, err := s.Run(context.Background(), domain.SourceMarkdown, "s.md#a", "spec"); err == nil {
		t.Fatal("方針の誤りはエラー")
	}

	policy := policyOf(t, `{"auto_approve": {"enabled": true, "llm_commands": ["m"], "runs": 2, "breaker": {"window": 5, "max_revoked": 1}}}`)
	s, llm, placer, ev, log := newAuto(t, policy, []domain.Claim{mdClaim("EX", "s.md#a", domain.KindExample, nil)}, nil)
	log.got = []domain.ReviewDecision{
		{Time: "2026-11-04T01:00:00Z", ID: "OLD", Verdict: domain.VerdictApprove, By: domain.AutoApproverName},
		{Time: "2026-11-04T02:00:00Z", ID: "OLD", Verdict: domain.VerdictRevoke, By: "nonchan7720"},
	}
	rep, err := s.Run(context.Background(), domain.SourceMarkdown, "s.md#a", "spec")
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Paused || placer.pending["EX"] != domain.AutoPaused || len(placer.approved) != 0 || ev["EX"] == nil {
		t.Fatalf("ブレーカーが止めていれば承認せず、抽出と証拠の記録は行う: %+v %+v", rep, placer)
	}
	if len(llm.commands) != 2 {
		t.Errorf("止まっているときは逆変換を呼ばない: %v", llm.commands)
	}
}

// 既にある ID の候補は、判定が ok でなくても既存の証拠を上書きしない（上書きすると既存の自動承認が unapproved になる）。
func TestAutoApprove_KeepsExistingEvidence(t *testing.T) {
	policy := policyOf(t, `{"auto_approve": {"enabled": true, "llm_commands": ["m"], "runs": 2, "kinds": ["manual"]}}`)
	cand := mdClaim("EX", "s.md#a", domain.KindExample, nil)
	s, _, placer, ev, _ := newAuto(t, policy, []domain.Claim{cand}, []domain.Claim{mdClaim("EX", "s.md#a", domain.KindExample, approved("h1"))})
	ev["EX"] = []byte(`{"id": "EX", "verdict": "ok"}`)
	if _, err := s.Run(context.Background(), domain.SourceMarkdown, "s.md#a", "spec"); err != nil {
		t.Fatal(err)
	}
	if placer.pending["EX"] != domain.AutoKind || string(ev["EX"]) != `{"id": "EX", "verdict": "ok"}` {
		t.Fatalf("既にある ID の証拠は書き換えない: %q %s", placer.pending["EX"], ev["EX"])
	}
}
