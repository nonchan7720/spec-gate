package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/services"
)

func TestHookResult_PassIsSilentExit0(t *testing.T) {
	rep := services.CheckReport{Claims: domain.Results{{ID: "A-1", Status: domain.StatusPass}}}
	code, msg := hookResult(rep)
	if code != 0 || msg != "" {
		t.Fatalf("pass なら exit 0 で何も出さない: code=%d msg=%q", code, msg)
	}
}

func TestHookResult_UnverifiedIsNotBlocking(t *testing.T) {
	rep := services.CheckReport{Claims: domain.Results{{ID: "M-1", Status: domain.StatusUnverified}}}
	if code, _ := hookResult(rep); code != 0 {
		t.Fatalf("unverified はエージェントを止めない: code=%d", code)
	}
}

func TestHookError_BlocksWithReason(t *testing.T) {
	// ハーネス自体が判定できなかったときに exit 0 で通すと「検証していないのに通った」になる。
	// 止め続けるループは gate（feedback-gate）が 5 回で打ち切るので、ここでは止める。
	code, msg := hookError(errors.New("claims/spec/A.yaml: yaml: line 3: mapping values are not allowed"))
	if code != 2 {
		t.Fatalf("内部エラーは exit 2 で止める: code=%d", code)
	}
	for _, want := range []string{"spec-gate", "判定できなかった", "claims/spec/A.yaml"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("メッセージに %q が無い: %q", want, msg)
		}
	}
}

func TestHookResult_BlockingAlwaysExit2(t *testing.T) {
	rep := services.CheckReport{Claims: domain.Results{
		{ID: "A-1", Status: domain.StatusPass, Title: "ok"},
		{ID: "A-2", Status: domain.StatusFail, Title: "ng", Detail: "期待=1 実際=2"},
	}}
	// ループ防止は gate（feedback-gate）側に任せるので、何度呼んでも諦めずに止める
	for i := range 5 {
		code, msg := hookResult(rep)
		if code != 2 {
			t.Fatalf("%d 回目: blocking なら exit 2: code=%d", i+1, code)
		}
		if !strings.Contains(msg, "A-2") || strings.Contains(msg, "A-1") {
			t.Fatalf("直すべきものだけ出す（pass は出さない）: %q", msg)
		}
	}
}
