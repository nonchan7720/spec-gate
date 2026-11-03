package services

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

const delegRef = "docs/spec/a.md#端数 処理"
const delegDir = ".spec-gate-llm/extract/docs_spec_a.md_端数_処理"

func newDelegExtract(work *fakeWork) (*ExtractService, *pendingStore) {
	store := &pendingStore{}
	md := fakeAdapter{typ: domain.SourceMarkdown, snaps: map[string]domain.Snapshot{
		delegRef:  {Excerpt: "金額の端数は切り捨てる"},
		"b.md":    {Excerpt: "仕様 b"},
		"gone.md": {Skipped: "見出しが無い"},
	}}
	return &ExtractService{
		Claims:   store,
		Adapters: map[domain.SourceType]domain.SourceAdapter{domain.SourceMarkdown: md},
		Prompter: fakeExtractPrompter{},
		Work:     work,
	}, store
}

// emit は run ごとに自己完結したプロンプトを書き、そのパスを返す。LLM（Extractor）は呼ばない。
func TestExtractEmit_WritesOneSelfContainedPromptPerRun(t *testing.T) {
	work := newFakeWork()
	svc, store := newDelegExtract(work)
	paths, err := svc.Emit(context.Background(), domain.SourceMarkdown, delegRef, 3)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/abs/" + delegDir + "/run-1.prompt.md",
		"/abs/" + delegDir + "/run-2.prompt.md",
		"/abs/" + delegDir + "/run-3.prompt.md",
	}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %v", paths)
	}
	for k, p := range ExtractPromptPaths(delegRef, 3) {
		body, ok := work.files[p]
		if !ok {
			t.Fatalf("%s が無い", p)
		}
		answer := "/abs/" + delegDir + "/run-" + string(rune('1'+k)) + ".answer.md"
		for _, s := range []string{"RULES(full)", "REF=" + delegRef, "SPEC=金額の端数は切り捨てる", answer} {
			if !strings.Contains(body, s) {
				t.Fatalf("%s に %q が無い（自己完結していない）:\n%s", p, s, body)
			}
		}
		// 他の run の回答ファイルを指さない
		for j := 1; j <= 3; j++ {
			other := "run-" + string(rune('0'+j)) + ".answer.md"
			if j != k+1 && strings.Contains(body, other) {
				t.Fatalf("%s が別 run の回答 %s を含む", p, other)
			}
		}
	}
	if len(store.pending) != 0 {
		t.Fatal("emit は claims/_pending に何も置かない")
	}
}

func TestExtractEmit_UnreadableSpecIsError(t *testing.T) {
	svc, _ := newDelegExtract(newFakeWork())
	if _, err := svc.Emit(context.Background(), domain.SourceMarkdown, "gone.md", 2); err == nil {
		t.Fatal("仕様が読めなければ error")
	}
}

// 2 回目の emit は前回の回答を消してから書く（古い回答を取り込まない）。
func TestExtractEmit_ClearsPreviousAnswers(t *testing.T) {
	work := newFakeWork()
	svc, _ := newDelegExtract(work)
	if _, err := svc.Emit(context.Background(), domain.SourceMarkdown, delegRef, 2); err != nil {
		t.Fatal(err)
	}
	work.files[delegDir+"/run-1.answer.md"] = "OLD|古い"
	if _, err := svc.Emit(context.Background(), domain.SourceMarkdown, delegRef, 2); err != nil {
		t.Fatal(err)
	}
	if _, ok := work.files[delegDir+"/run-1.answer.md"]; ok {
		t.Fatal("前回の回答が残っている")
	}
}

// 回答がそろえば、これまでと同じ合議で _pending に置き、その ref の作業ディレクトリを消す。
func TestExtractCollect_AllAnswersRunsConsensusAndCleansUp(t *testing.T) {
	work := newFakeWork()
	svc, store := newDelegExtract(work)
	if _, err := svc.Emit(context.Background(), domain.SourceMarkdown, delegRef, 3); err != nil {
		t.Fatal(err)
	}
	work.files[delegDir+"/run-1.answer.md"] = "R-1|端数は切り捨てる\nR-2|負数は扱わない"
	work.files[delegDir+"/run-2.answer.md"] = "R-1|端数は、切り捨てる"
	work.files[delegDir+"/run-3.answer.md"] = "R-9|端数は切り捨てる"

	cands, err := svc.Collect(context.Background(), delegRef)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 2 || cands[0].Claim.ID != "R-1" || cands[0].Agreements != 3 || cands[0].Runs != 3 {
		t.Fatalf("合議の結果: %+v", cands)
	}
	if cands[1].Claim.ID != "R-2" || cands[1].Agreements != 1 {
		t.Fatalf("1/3 の候補も残す: %+v", cands)
	}
	if !reflect.DeepEqual(store.conf, []string{"3/3", "1/3"}) {
		t.Fatalf("conf = %v", store.conf)
	}
	for _, c := range store.pending {
		if c.Source.Type != domain.SourceMarkdown || c.Source.Ref != delegRef {
			t.Fatalf("source は emit 時の type / ref: %+v", c.Source)
		}
	}
	if got := work.under(delegDir); len(got) != 0 {
		t.Fatalf("collect 後に作業ディレクトリが残っている: %v", got)
	}
}

// 回答が欠けていれば、欠けている run を挙げてエラー。何も置かず、作業ディレクトリも消さない。
func TestExtractCollect_MissingAnswersListsRuns(t *testing.T) {
	work := newFakeWork()
	svc, store := newDelegExtract(work)
	if _, err := svc.Emit(context.Background(), domain.SourceMarkdown, delegRef, 3); err != nil {
		t.Fatal(err)
	}
	work.files[delegDir+"/run-2.answer.md"] = "R-1|端数は切り捨てる"

	_, err := svc.Collect(context.Background(), delegRef)
	var miss *MissingAnswersError
	if !errors.As(err, &miss) {
		t.Fatalf("MissingAnswersError を返す: %v", err)
	}
	if !reflect.DeepEqual(miss.Missing, []string{"run-1", "run-3"}) {
		t.Fatalf("欠けた run = %v", miss.Missing)
	}
	if !strings.Contains(err.Error(), "run-1") || !strings.Contains(err.Error(), "/abs/"+delegDir+"/run-3.answer.md") {
		t.Fatalf("エラーに欠けた run と回答のパスを書く: %v", err)
	}
	if len(store.pending) != 0 {
		t.Fatal("欠けていれば何も置かない")
	}
	if _, ok := work.files[delegDir+"/run-2.answer.md"]; !ok {
		t.Fatal("欠けていれば作業ディレクトリを消さない")
	}
}

// 別の ref の回答は混ぜない。回答は collect した ref のディレクトリからだけ読む。
func TestExtractCollect_NeverMixesRefs(t *testing.T) {
	work := newFakeWork()
	svc, store := newDelegExtract(work)
	ctx := context.Background()
	if _, err := svc.Emit(ctx, domain.SourceMarkdown, delegRef, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Emit(ctx, domain.SourceMarkdown, "b.md", 2); err != nil {
		t.Fatal(err)
	}
	bDir := ".spec-gate-llm/extract/b.md"
	work.files[bDir+"/run-1.answer.md"] = "B-1|b の主張"
	work.files[bDir+"/run-2.answer.md"] = "B-1|b の主張"
	work.files[delegDir+"/run-1.answer.md"] = "R-1|a の主張"

	if _, err := svc.Collect(ctx, delegRef); err == nil {
		t.Fatal("a の run-2 が欠けているのに、b の回答で埋めてはいけない")
	}
	cands, err := svc.Collect(ctx, "b.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 1 || cands[0].Claim.ID != "B-1" || cands[0].Agreements != 2 {
		t.Fatalf("b は b の回答だけ: %+v", cands)
	}
	for _, c := range store.pending {
		if c.ID == "R-1" {
			t.Fatal("a の回答が b に混ざった")
		}
	}
	if _, ok := work.files[delegDir+"/run-1.answer.md"]; !ok {
		t.Fatal("b の collect が a の作業ディレクトリを消した")
	}
}

func TestExtractCollect_WithoutEmitIsError(t *testing.T) {
	svc, _ := newDelegExtract(newFakeWork())
	if _, err := svc.Collect(context.Background(), delegRef); err == nil || !strings.Contains(err.Error(), "--emit") {
		t.Fatalf("emit していなければ案内つきの error: %v", err)
	}
}

// 置き換え後の名前が同じになる別の ref（a/b と a_b）は、同じディレクトリを共有しない。
func TestExtractEmit_RefNameCollisionIsError(t *testing.T) {
	work := newFakeWork()
	svc, _ := newDelegExtract(work)
	svc.Adapters[domain.SourceMarkdown].(fakeAdapter).snaps["a/b"] = domain.Snapshot{Excerpt: "x"}
	svc.Adapters[domain.SourceMarkdown].(fakeAdapter).snaps["a_b"] = domain.Snapshot{Excerpt: "y"}
	ctx := context.Background()
	if _, err := svc.Emit(ctx, domain.SourceMarkdown, "a/b", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Emit(ctx, domain.SourceMarkdown, "a_b", 1); err == nil {
		t.Fatal("名前が衝突する別 ref の emit は error")
	}
	if _, err := svc.Collect(ctx, "a_b"); err == nil {
		t.Fatal("名前が衝突する別 ref の collect は error")
	}
}

// 回答が読めなければ、どの run かを示してエラー（合議に混ぜない）。
func TestExtractCollect_UnparsableAnswerIsError(t *testing.T) {
	work := newFakeWork()
	svc, store := newDelegExtract(work)
	if _, err := svc.Emit(context.Background(), domain.SourceMarkdown, delegRef, 2); err != nil {
		t.Fatal(err)
	}
	work.files[delegDir+"/run-1.answer.md"] = "R-1|ok"
	work.files[delegDir+"/run-2.answer.md"] = "壊れた回答"
	_, err := svc.Collect(context.Background(), delegRef)
	if err == nil || !strings.Contains(err.Error(), "run-2") {
		t.Fatalf("壊れた run を示す: %v", err)
	}
	if len(store.pending) != 0 {
		t.Fatal("壊れた回答があれば何も置かない")
	}
}
