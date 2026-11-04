package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/spf13/cobra"
)

// guardExitDeny は PreToolUse hook で操作を止める終了コード（Claude Code は exit 2 の stderr をエージェントに返す）。
const guardExitDeny = 2

func newGuardCmd(a **app) *cobra.Command {
	return &cobra.Command{
		Use:   "guard",
		Short: "PreToolUse hook 用: 承認役だけが auto-approve を実行でき、revoke と保護するパスの書き換えを拒否する",
		Long: `stdin に PreToolUse hook の JSON（tool_name・tool_input・agent_type）を受け、操作を許すかを決める
（docs/spec/auto-approval.md「承認役のエージェント」「保護の範囲」）。
許すなら何も出さず exit 0、拒否するなら理由を stderr に出して exit 2。入力が読めないときも拒否する。
.claude/settings.json の PreToolUse（matcher Bash|Edit|Write|MultiEdit|NotebookEdit）と、
承認役（.claude/agents/spec-gate-approver.md）の frontmatter の hooks から呼ぶ。`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if code := runGuard(os.Stdin, os.Stderr, (*a).root); code != 0 {
				os.Exit(code)
			}
			return nil
		},
	}
}

// guardHookInput は PreToolUse hook の stdin のうち、判定に使う部分。
type guardHookInput struct {
	ToolName  string `json:"tool_name"`
	AgentType string `json:"agent_type"`
	ToolInput struct {
		Command      string `json:"command"`
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
	} `json:"tool_input"`
}

// runGuard は hook の stdin を判定し、終了コードを返す（0 は許可、2 は拒否）。
func runGuard(stdin io.Reader, stderr io.Writer, root string) int {
	var in guardHookInput
	if err := json.NewDecoder(stdin).Decode(&in); err != nil {
		_, _ = fmt.Fprintln(stderr, "spec-gate guard: hook の入力（JSON）が読めないので止める:", err)
		return guardExitDeny
	}
	path := in.ToolInput.FilePath
	if path == "" {
		path = in.ToolInput.NotebookPath
	}
	d := domain.Guard(domain.GuardInput{Tool: in.ToolName, Command: in.ToolInput.Command, Path: path, AgentType: in.AgentType}, root)
	if d.Verdict == domain.GuardAllow {
		return 0
	}
	_, _ = fmt.Fprintln(stderr, "spec-gate guard:", d.Reason)
	return guardExitDeny
}
