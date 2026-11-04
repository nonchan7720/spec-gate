package domain

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// 保護の判定（docs/spec/auto-approval.md「承認役のエージェント」「保護の範囲」）。PreToolUse hook の spec-gate guard が使う。

// ApproverAgentType は承認役のサブエージェントの名前（hook の入力の agent_type）。
const ApproverAgentType = "spec-gate-approver"

// 判定の結果。
const (
	GuardAllow = "allow"
	GuardDeny  = "deny"
)

// GuardInput は判定する操作。Command は Bash、Path は Edit / Write のときに入る。AgentType は無ければ空。
type GuardInput struct {
	Tool      string `json:"tool"`
	Command   string `json:"command,omitempty"`
	Path      string `json:"path,omitempty"`
	AgentType string `json:"agent_type,omitempty"`
}

// GuardDecision は判定と、拒否の理由（hook が stderr に出す）。
type GuardDecision struct {
	Verdict string
	Reason  string
}

func allow() GuardDecision { return GuardDecision{Verdict: GuardAllow} }

func deny(reason string) GuardDecision { return GuardDecision{Verdict: GuardDeny, Reason: reason} }

// protectedExact と protectedDirs は、エージェントが書き換えてはいけないパス（プロジェクトルートからの相対）。
var (
	protectedExact = []string{".claude/gate.yaml", ".claude/agents/spec-gate-approver.md", ".claude/settings.json", ".claude/settings.local.json"}
	protectedDirs  = []string{".spec-gate", ".claude/feedback"}
)

// IsProtectedPath は path（root からの相対か絶対）が保護するパスか。root が空なら path を相対とみなす。
func IsProtectedPath(path, root string) bool {
	p := filepath.ToSlash(filepath.Clean(path))
	if root != "" && filepath.IsAbs(path) {
		rel, err := filepath.Rel(root, path)
		if err != nil || strings.HasPrefix(rel, "..") {
			return false
		}
		p = filepath.ToSlash(rel)
	}
	if slices.Contains(protectedExact, p) {
		return true
	}
	for _, d := range protectedDirs {
		if p == d || strings.HasPrefix(p, d+"/") {
			return true
		}
	}
	return false
}

// Guard は操作を許すかを決める。root は Edit / Write の絶対パスを相対にするためのプロジェクトルート（空なら相対とみなす）。
func Guard(in GuardInput, root string) GuardDecision {
	approver := in.AgentType == ApproverAgentType
	switch in.Tool {
	case "Bash":
		if approver {
			return guardApproverCommand(in.Command)
		}
		return guardCommand(in.Command, root)
	case "Edit", "Write", "MultiEdit", "NotebookEdit":
		if approver {
			return deny("承認役（spec-gate-approver）は Bash で spec-gate auto-approve / audit を実行するだけ。ファイルは書き換えない")
		}
		if IsProtectedPath(in.Path, root) {
			return deny(in.Path + " は保護しているので書き換えない（hook-gate の規則・承認役の定義・hook の設定・.spec-gate/）")
		}
		return allow()
	}
	if approver {
		return deny("承認役（spec-gate-approver）が使えるのは Bash だけ")
	}
	return allow()
}

// shellMeta はコマンドをつなぐ・置き換える記号。承認役のコマンドにあれば拒否する。
var shellMeta = regexp.MustCompile("[;&|<>`\n]|\\$\\(")

// guardApproverCommand は、承認役が実行してよい spec-gate auto-approve / audit だけを 1 つ許す。
func guardApproverCommand(command string) GuardDecision {
	if shellMeta.MatchString(command) {
		return deny("承認役のコマンドは spec-gate auto-approve / audit を 1 つだけ（; & | や置き換えでつながない）")
	}
	if sub, ok := specGateSubcommand(strings.Fields(command)); ok && (sub == "auto-approve" || sub == "audit") {
		return allow()
	}
	return deny("承認役が実行してよいのは spec-gate auto-approve と spec-gate audit だけ")
}

// guardCommand は承認役でないエージェントの Bash を確かめる。auto-approve（承認役だけ）・revoke（人間だけ）と、保護するパスへのシェル経由の書き込みを拒否する。
func guardCommand(command, root string) GuardDecision {
	for _, seg := range splitCommands(command) {
		fields := strings.Fields(seg)
		switch sub, _ := specGateSubcommand(fields); sub {
		case "auto-approve":
			return deny("spec-gate auto-approve は承認役（spec-gate-approver サブエージェント）だけが実行する")
		case "revoke":
			return deny("spec-gate revoke（承認の取り消し）は人間の操作。ユーザーに依頼すること")
		}
		if target, ok := shellWriteTarget(seg, fields, root); ok {
			return deny(target + " は保護しているのでシェル経由でも書き換えない")
		}
	}
	return allow()
}

// splitCommands はコマンドを ; & | 改行で区切る（引用符の中までは見ない。区切りすぎは拒否の側に倒れる）。
func splitCommands(command string) []string {
	return regexp.MustCompile(`[;&|\n]+`).Split(command, -1)
}

// specGateSubcommand は spec-gate を呼ぶコマンドのサブコマンドを返す。先頭の環境変数・env・mise exec -- と、
// -C / --root とその値を飛ばす。spec-gate（パス付きも）と go run <パッケージ> を spec-gate とみなす。
func specGateSubcommand(fields []string) (string, bool) {
	i := 0
	for i < len(fields) {
		f := fields[i]
		if f == "env" || (strings.Contains(f, "=") && !strings.HasPrefix(f, "-")) {
			i++
			continue
		}
		if f == "mise" && i+2 < len(fields) && (fields[i+1] == "exec" || fields[i+1] == "x") && fields[i+2] == "--" {
			i += 3
			continue
		}
		break
	}
	switch {
	case i < len(fields) && strings.TrimSuffix(filepath.Base(strings.Trim(fields[i], `"'`)), ".exe") == "spec-gate":
		i++
	case i+2 < len(fields) && fields[i] == "go" && fields[i+1] == "run":
		i += 3
	default:
		return "", false
	}
	for i < len(fields) {
		switch f := fields[i]; {
		case f == "-C" || f == "--root":
			i += 2
		case strings.HasPrefix(f, "-"):
			i++
		default:
			return f, true
		}
	}
	return "", false
}

var redirectTarget = regexp.MustCompile(`>{1,2}\s*["']?([^\s"';&|]+)`)

// writeCommands は、引数のパスを書き換えるコマンド（claims/** の保護と同じ顔ぶれ）。
var writeCommands = map[string]bool{"tee": true, "mv": true, "cp": true, "rm": true, "truncate": true}

// shellWriteTarget は、区切った 1 つのコマンドが保護するパスへ書き込むなら、そのパスを返す。
func shellWriteTarget(seg string, fields []string, root string) (string, bool) {
	for _, m := range redirectTarget.FindAllStringSubmatch(seg, -1) {
		if IsProtectedPath(m[1], root) {
			return m[1], true
		}
	}
	if len(fields) == 0 {
		return "", false
	}
	cmd := filepath.Base(fields[0])
	branch := len(fields) > 2 && (fields[2] == "-b" || fields[2] == "-B") // git checkout -b の引数はブランチ名
	writes := writeCommands[cmd] ||
		(cmd == "sed" && len(fields) > 1 && strings.HasPrefix(fields[1], "-i")) ||
		(cmd == "git" && len(fields) > 1 && (fields[1] == "restore" || (fields[1] == "checkout" && !branch)))
	if !writes {
		return "", false
	}
	for _, f := range fields[1:] {
		if a := strings.Trim(f, `"'`); IsProtectedPath(a, root) {
			return a, true
		}
	}
	return "", false
}
