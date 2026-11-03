package services

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

const revDir = ".spec-gate-llm/reverse"

// fakeReversePrompter は決まった形のプロンプトを作り、比較の回答は "match" / "diff:<食い違い>" として読む。
type fakeReversePrompter struct{}

func (fakeReversePrompter) RestatePrompt(c domain.Claim) (string, error) {
	return "RESTATE-RULES\nCLAIM=" + c.ID + " " + c.Title, nil
}

func (fakeReversePrompter) ParseRestate(answer string) (string, error) {
	if s := strings.TrimSpace(answer); s != "" {
		return s, nil
	}
	return "", errors.New("再生成した仕様文が空")
}

func (fakeReversePrompter) ComparePrompt(restated, original string) string {
	return "COMPARE-RULES\nA=" + restated + "\nB=" + original
}

func (fakeReversePrompter) ParseCompare(answer string) (domain.Comparison, error) {
	s := strings.TrimSpace(answer)
	if s == "match" {
		return domain.Comparison{Match: true}, nil
	}
	if d, ok := strings.CutPrefix(s, "diff:"); ok {
		return domain.Comparison{Differences: []string{d}}, nil
	}
	return domain.Comparison{}, errors.New("JSON が無い")
}

func newDelegReverse(work *fakeWork) *ReverseService {
	svc := newReverseFixture(&fakeReverser{})
	svc.Reverser = nil // 委譲では LLM を呼ばない
	svc.Prompter = fakeReversePrompter{}
	svc.Work = work
	return svc
}

// emit は再生成のプロンプトを、テキストを持つ claim ごとに 1 つ書く。元の仕様は含めない。
func TestReverseEmit_WritesRestatePromptPerClaim(t *testing.T) {
	work := newFakeWork()
	svc := newDelegReverse(work)
	d, err := svc.Emit(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/abs/" + revDir + "/A-1.restate.prompt.md", "/abs/" + revDir + "/A-2.restate.prompt.md"}
	if d.Phase != ReversePhaseRestate || !reflect.DeepEqual(d.Prompts, want) {
		t.Fatalf("emit = %+v", d)
	}
	body := work.files[revDir+"/A-1.restate.prompt.md"]
	for _, s := range []string{"RESTATE-RULES", "CLAIM=A-1", "/abs/" + revDir + "/A-1.restate.answer.md"} {
		if !strings.Contains(body, s) {
			t.Fatalf("A-1 の再生成プロンプトに %q が無い:\n%s", s, body)
		}
	}
	if strings.Contains(body, "仕様 a") || strings.Contains(body, "A-2") {
		t.Fatalf("再生成プロンプトに元の仕様や他の claim を入れない:\n%s", body)
	}
}

func TestReverseEmit_UnknownIDIsError(t *testing.T) {
	svc := newDelegReverse(newFakeWork())
	if _, err := svc.Emit(context.Background(), []string{"NOPE-1"}); err == nil {
		t.Fatal("存在しない claim は error")
	}
}

func TestReverseCollect_MissingRestateAnswersListsClaims(t *testing.T) {
	work := newFakeWork()
	svc := newDelegReverse(work)
	ctx := context.Background()
	if _, err := svc.Emit(ctx, nil); err != nil {
		t.Fatal(err)
	}
	work.files[revDir+"/A-1.restate.answer.md"] = "円未満は切り捨てる。"
	_, err := svc.Collect(ctx, nil)
	var miss *MissingAnswersError
	if !errors.As(err, &miss) || !reflect.DeepEqual(miss.Missing, []string{"A-2.restate"}) {
		t.Fatalf("欠けた回答を挙げる: %v", err)
	}
	if _, ok := work.files[revDir+"/A-1.compare.prompt.md"]; ok {
		t.Fatal("欠けているうちは比較のプロンプトを書かない")
	}
}

// 2 段階: 再生成の回答がそろえば比較のプロンプト、比較の回答がそろえばレポートを出して消す。
func TestReverseCollect_TwoPhases(t *testing.T) {
	work := newFakeWork()
	svc := newDelegReverse(work)
	ctx := context.Background()
	if _, err := svc.Emit(ctx, nil); err != nil {
		t.Fatal(err)
	}
	work.files[revDir+"/A-1.restate.answer.md"] = "円未満は切り捨てる。\n"
	work.files[revDir+"/A-2.restate.answer.md"] = "負数は 0 にする。"

	d, err := svc.Collect(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/abs/" + revDir + "/A-1.compare.prompt.md", "/abs/" + revDir + "/A-2.compare.prompt.md"}
	if d.Phase != ReversePhaseCompare || !reflect.DeepEqual(d.Prompts, want) || d.Report != nil {
		t.Fatalf("collect(1) = %+v", d)
	}
	body := work.files[revDir+"/A-1.compare.prompt.md"]
	for _, s := range []string{"COMPARE-RULES", "A=円未満は切り捨てる。", "B=仕様 a", "/abs/" + revDir + "/A-1.compare.answer.md"} {
		if !strings.Contains(body, s) {
			t.Fatalf("比較プロンプトに %q が無い:\n%s", s, body)
		}
	}
	if strings.Contains(body, "負数は 0 にする") || strings.Contains(body, "仕様 b") {
		t.Fatalf("比較プロンプトに他の claim の内容を入れない:\n%s", body)
	}

	// 比較の回答が欠けていればエラー
	work.files[revDir+"/A-1.compare.answer.md"] = "match"
	var miss *MissingAnswersError
	if _, err := svc.Collect(ctx, nil); !errors.As(err, &miss) || !reflect.DeepEqual(miss.Missing, []string{"A-2.compare"}) {
		t.Fatalf("比較の欠けを挙げる: %v", err)
	}

	work.files[revDir+"/A-2.compare.answer.md"] = "diff:負数の扱いが仕様に無い"
	d, err = svc.Collect(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if d.Phase != ReversePhaseDone || d.Report == nil || len(d.Prompts) != 0 {
		t.Fatalf("collect(2) = %+v", d)
	}
	got := byID(*d.Report)
	if got["A-1"].Status != domain.ReverseMatch || got["A-1"].Restated != "円未満は切り捨てる。" || got["A-1"].Spec != "spec.md:1" {
		t.Fatalf("A-1 = %+v", got["A-1"])
	}
	if got["A-2"].Status != domain.ReverseMismatch || !reflect.DeepEqual(got["A-2"].Differences, []string{"負数の扱いが仕様に無い"}) {
		t.Fatalf("A-2 = %+v", got["A-2"])
	}
	if got["A-3"].Status != domain.ReverseSkipped || got["UI-1"].Status != domain.ReverseSkipped {
		t.Fatalf("読めない仕様 / figma は skipped: %+v", got)
	}
	var ids []string
	for _, r := range d.Report.Results {
		ids = append(ids, r.ID)
	}
	if !reflect.DeepEqual(ids, []string{"A-1", "A-2", "A-3", "UI-1"}) {
		t.Fatalf("claim の順に並べる: %v", ids)
	}
	if left := work.under(revDir); len(left) != 0 {
		t.Fatalf("レポート後に作業ディレクトリが残っている: %v", left)
	}
}

// 再生成の回答が空なら、その claim は error にして比較に進めない。
func TestReverseCollect_EmptyRestateIsErrorAndSkipsCompare(t *testing.T) {
	work := newFakeWork()
	svc := newDelegReverse(work)
	ctx := context.Background()
	if _, err := svc.Emit(ctx, []string{"A-1", "A-2"}); err != nil {
		t.Fatal(err)
	}
	work.files[revDir+"/A-1.restate.answer.md"] = "  \n"
	work.files[revDir+"/A-2.restate.answer.md"] = "負数は 0 にする。"
	d, err := svc.Collect(ctx, []string{"A-1", "A-2"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Prompts, []string{"/abs/" + revDir + "/A-2.compare.prompt.md"}) {
		t.Fatalf("A-1 は比較しない: %+v", d)
	}
	work.files[revDir+"/A-2.compare.answer.md"] = "解釈できない応答"
	d, err = svc.Collect(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := byID(*d.Report)
	if got["A-1"].Status != domain.ReverseError || !strings.Contains(got["A-1"].Detail, "空") {
		t.Fatalf("A-1 = %+v", got["A-1"])
	}
	if got["A-2"].Status != domain.ReverseError {
		t.Fatalf("解釈できない比較は一致とみなさない: %+v", got["A-2"])
	}
}

// emit したときと違う claim を指定した collect は、取り違えなので error。
func TestReverseCollect_DifferentIDsIsError(t *testing.T) {
	work := newFakeWork()
	svc := newDelegReverse(work)
	ctx := context.Background()
	if _, err := svc.Emit(ctx, []string{"A-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Collect(ctx, []string{"A-2"}); err == nil {
		t.Fatal("emit と違う claim の collect は error")
	}
}

func TestReverseCollect_WithoutEmitIsError(t *testing.T) {
	svc := newDelegReverse(newFakeWork())
	if _, err := svc.Collect(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "--emit") {
		t.Fatalf("emit していなければ案内つきの error: %v", err)
	}
}
