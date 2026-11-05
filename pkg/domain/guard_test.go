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

// 自己レビューで見つかった抜け道（2026-11-04）。
func TestGuard_Bypasses(t *testing.T) {
	root := "/repo"
	approver := func(cmd string) GuardInput {
		return GuardInput{Tool: "Bash", Command: cmd, AgentType: ApproverAgentType}
	}
	agent := func(cmd string) GuardInput { return GuardInput{Tool: "Bash", Command: cmd} }
	cases := []struct {
		name string
		in   GuardInput
		want string
	}{
		{"承認役の go run", approver("go run /tmp/x.go audit"), GuardDeny},
		{"承認役の環境変数の前置き", approver("PATH=/tmp/evil spec-gate auto-approve a.md#x --area auto"), GuardDeny},
		{"承認役の LD_PRELOAD", approver("LD_PRELOAD=/tmp/x.so spec-gate audit"), GuardDeny},
		{"承認役のほかの場所の spec-gate", approver("/tmp/evil/spec-gate audit"), GuardDeny},
		{"承認役のルートの bin", approver("/repo/bin/spec-gate -C /repo auto-approve a.md#x --area auto"), GuardAllow},
		{"$(…) の中の revoke", agent("echo $(spec-gate revoke AUTO-001 --reason x)"), GuardDeny},
		{"バッククォートの中の revoke", agent("echo `spec-gate revoke AUTO-001 --reason x`"), GuardDeny},
		{"bash -c の auto-approve", agent("bash -c 'spec-gate auto-approve docs/a.md#x --area auto'"), GuardDeny},
		{"sh -lc の revoke", agent(`sh -lc "go run . revoke X --reason y"`), GuardDeny},
		{"eval の revoke", agent("eval 'spec-gate revoke X --reason y'"), GuardDeny},
		{"sudo cp で保護パス", agent("sudo cp x .spec-gate/policy.yaml"), GuardDeny},
		{"sudo -u の後ろ", agent("sudo -u root spec-gate revoke X --reason y"), GuardDeny},
		{"timeout の後ろ", agent("timeout 10 spec-gate auto-approve a.md#x"), GuardDeny},
		{"go run のフラグの値", agent("go run -tags t . revoke X --reason y"), GuardDeny},
		{"go run の = つきフラグ", agent("go run -tags=t . auto-approve a.md#x"), GuardDeny},
		{"xargs の後ろ", agent("echo X | xargs spec-gate revoke --reason y"), GuardDeny},
		{"関係ない go run", agent("go run -tags t ./tools/bind guard"), GuardAllow},
		{"grep での言及は通す", agent(`grep -rn "auto-approve" docs`), GuardAllow},
		{"関係ない bash -c", agent("bash -c 'go test ./...'"), GuardAllow},
	}
	for _, c := range cases {
		if got := Guard(c.in, root); got.Verdict != c.want {
			t.Errorf("%s: %q = %s (%s), want %s", c.name, c.in.Command, got.Verdict, got.Reason, c.want)
		}
	}
}
