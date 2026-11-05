package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestGuardHook(t *testing.T) {
	root := "/repo"
	cases := []struct {
		name, stdin string
		code        int
	}{
		{"通常のコマンド", `{"tool_name": "Bash", "tool_input": {"command": "go test ./..."}}`, 0},
		{"メインからの auto-approve", `{"tool_name": "Bash", "tool_input": {"command": "spec-gate auto-approve a.md#x --area auto"}}`, 2},
		{"承認役の auto-approve", `{"tool_name": "Bash", "agent_type": "spec-gate-approver", "tool_input": {"command": "spec-gate auto-approve a.md#x --area auto"}}`, 0},
		{"ほかのサブエージェントの auto-approve", `{"tool_name": "Bash", "agent_type": "general-purpose", "tool_input": {"command": "spec-gate auto-approve a.md#x --area auto"}}`, 2},
		{"絶対パスで保護パスを Edit", `{"tool_name": "Edit", "tool_input": {"file_path": "/repo/.claude/gate.yaml"}}`, 2},
		{"絶対パスで通常のファイルを Write", `{"tool_name": "Write", "tool_input": {"file_path": "/repo/pkg/a.go"}}`, 0},
		{"NotebookEdit の保護パス", `{"tool_name": "NotebookEdit", "tool_input": {"notebook_path": "/repo/.spec-gate/x.ipynb"}}`, 2},
		{"読めない入力", `not json`, 2},
	}
	for _, c := range cases {
		var errw bytes.Buffer
		code := runGuard(strings.NewReader(c.stdin), &errw, root)
		if code != c.code {
			t.Errorf("%s: exit %d, want %d (%s)", c.name, code, c.code, errw.String())
		}
		if code == 0 && errw.Len() != 0 {
			t.Errorf("%s: 許可なら何も出さない: %q", c.name, errw.String())
		}
		if code != 0 && !strings.HasPrefix(errw.String(), "spec-gate guard: ") {
			t.Errorf("%s: 拒否の理由を stderr に出す: %q", c.name, errw.String())
		}
	}
}
