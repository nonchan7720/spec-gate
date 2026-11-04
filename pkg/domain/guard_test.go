package domain

import "testing"

// 仕様の表（tools/bind の TestGuard）に無い書き方も、同じ規則で判定する。
func TestGuard_Variants(t *testing.T) {
	root := "/repo"
	cases := []struct {
		name string
		in   GuardInput
		want string
	}{
		{"絶対パスの保護パス", GuardInput{Tool: "Edit", Path: "/repo/.claude/feedback/x.md"}, GuardDeny},
		{"絶対パスの通常のファイル", GuardInput{Tool: "Write", Path: "/repo/pkg/a.go"}, GuardAllow},
		{"ルートの外", GuardInput{Tool: "Write", Path: "/tmp/.spec-gate/x"}, GuardAllow},
		{"./ 付きの保護パス", GuardInput{Tool: "Edit", Path: "./.spec-gate/policy.yaml"}, GuardDeny},
		{"mise exec 経由の auto-approve", GuardInput{Tool: "Bash", Command: "mise exec -- go run . auto-approve docs/a.md#x --area auto"}, GuardDeny},
		{"環境変数と -C を挟んだ revoke", GuardInput{Tool: "Bash", Command: "FOO=1 bin/spec-gate -C . revoke X --reason y"}, GuardDeny},
		{"つないだ先の auto-approve", GuardInput{Tool: "Bash", Command: "go test ./... && spec-gate auto-approve a.md#x --area auto"}, GuardDeny},
		{"sed -i で保護パス", GuardInput{Tool: "Bash", Command: "sed -i 's/a/b/' .claude/gate.yaml"}, GuardDeny},
		{"git checkout で保護パスを戻す", GuardInput{Tool: "Bash", Command: "git checkout -- .claude/settings.json"}, GuardDeny},
		{"git checkout -b はブランチ名", GuardInput{Tool: "Bash", Command: "git checkout -b .spec-gate-fix"}, GuardAllow},
		{"保護パスを読むだけ", GuardInput{Tool: "Bash", Command: "grep -n x .spec-gate/review-decisions.jsonl"}, GuardAllow},
		{"承認役のパス付き spec-gate", GuardInput{Tool: "Bash", Command: "bin/spec-gate auto-approve a.md#x --area auto", AgentType: ApproverAgentType}, GuardAllow},
		{"承認役の置き換え", GuardInput{Tool: "Bash", Command: "spec-gate audit $(rm -rf claims)", AgentType: ApproverAgentType}, GuardDeny},
		{"承認役の Read", GuardInput{Tool: "Read", Path: "README.md", AgentType: ApproverAgentType}, GuardDeny},
		{"承認役でない Read", GuardInput{Tool: "Read", Path: ".spec-gate/policy.yaml"}, GuardAllow},
	}
	for _, c := range cases {
		if got := Guard(c.in, root); got.Verdict != c.want {
			t.Errorf("%s: %+v = %s (%s), want %s", c.name, c.in, got.Verdict, got.Reason, c.want)
		}
	}
}

func TestGuard_DenyHasReason(t *testing.T) {
	if d := Guard(GuardInput{Tool: "Bash", Command: "spec-gate revoke X --reason y"}, ""); d.Reason == "" {
		t.Fatal("拒否には理由を付ける（hook が stderr に出す）")
	}
}
