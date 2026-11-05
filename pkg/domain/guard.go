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
	protectedExact = []string{".claude/gate.yaml", ".claude/agents/spec-gate-approver.md", ".claude/settings.json", ".claude/settings.local.json",
		"bin/spec-gate", ".mcp.json", "scripts/mcp.sh"}
	protectedDirs = []string{".spec-gate", ".claude/feedback", "bin/.pinned"}
)

// relToRoot は path を root からの相対（スラッシュ区切り）にする。root の外なら false。root が空なら path を相対とみなす。
func relToRoot(path, root string) (string, bool) {
	p := filepath.ToSlash(filepath.Clean(path))
	if root != "" && filepath.IsAbs(path) {
		rel, err := filepath.Rel(root, path)
		if err != nil || strings.HasPrefix(rel, "..") {
			return "", false
		}
		p = filepath.ToSlash(rel)
	}
	return p, true
}

// IsProtectedPath は path（root からの相対か絶対）が保護するパスか。root が空なら path を相対とみなす。
func IsProtectedPath(path, root string) bool {
	p, ok := relToRoot(path, root)
	if !ok {
		return false
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

// coversProtected は path が保護するパスか、保護するパスを含むディレクトリ（bin・.claude など。ルートそのものは除く）か。
// rm -rf bin・mv bin x・go build -o bin/ のように、親を消す・置き換える操作を止めるのに使う。
func coversProtected(path, root string) bool {
	if IsProtectedPath(path, root) {
		return true
	}
	p, ok := relToRoot(path, root)
	if !ok || p == "." || p == "" {
		return false
	}
	for _, e := range slices.Concat(protectedExact, protectedDirs) {
		if strings.HasPrefix(e, p+"/") {
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
			return guardApproverCommand(in.Command, root)
		}
		return guardCommand(in.Command, root)
	case "Edit", "Write", "MultiEdit", "NotebookEdit":
		if approver {
			return deny("承認役（spec-gate-approver）は Bash で spec-gate auto-approve / audit を実行するだけ。ファイルは書き換えない")
		}
		if IsProtectedPath(in.Path, root) || outsideSpecGate(in.Path, root) {
			return deny(in.Path + " は保護しているので書き換えない（hook-gate の規則・承認役の定義・hook の設定・固定版の spec-gate の入口・.spec-gate/）")
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
// spec-gate のバイナリを前置きなしで実行するものに限る（go run・環境変数の前置き・ほかの場所の spec-gate は任意のコードを動かせる）。
func guardApproverCommand(command, root string) GuardDecision {
	if shellMeta.MatchString(command) {
		return deny("承認役のコマンドは spec-gate auto-approve / audit を 1 つだけ（; & | や置き換えでつながない）")
	}
	f := strings.Fields(command)
	if len(f) == 0 || !isSpecGateBinary(f[0], root) {
		return deny("承認役は spec-gate のバイナリ（spec-gate・bin/spec-gate・<ルート>/bin/spec-gate）を前置きなしで実行するだけ")
	}
	if sub := subcommandAfter(f[1:]); sub == "auto-approve" || sub == "audit" {
		return allow()
	}
	return deny("承認役が実行してよいのは spec-gate auto-approve と spec-gate audit だけ")
}

// isSpecGateBinary は、承認役が実行してよい spec-gate のバイナリか。
func isSpecGateBinary(f, root string) bool {
	switch f {
	case "spec-gate", "bin/spec-gate", "./bin/spec-gate":
		return true
	}
	return root != "" && f == filepath.Join(root, "bin", "spec-gate")
}

// guardCommand は承認役でないエージェントの Bash を確かめる。auto-approve（承認役だけ）・revoke（人間だけ）と、保護するパスへのシェル経由の書き込みを拒否する。
// $(…)・バッククォート・sh -c '…'・eval の中身と、sudo・env・time などの前置きの後ろも見る。
func guardCommand(command, root string) GuardDecision {
	for _, seg := range commandUnits(command, 0) {
		fields := unwrap(tokens(seg))
		switch sub, _ := specGateSubcommand(fields); sub {
		case "auto-approve":
			return deny("spec-gate auto-approve は承認役（spec-gate-approver サブエージェント）だけが実行する")
		case "revoke":
			return deny("spec-gate revoke（承認の取り消し）は人間の操作。ユーザーに依頼すること")
		case "review":
			// review --by x record のようにフラグを挟んでも record（hook が受ける人間の回答）。偽の stdin で承認を作れる
			if slices.Contains(fields, "record") {
				return deny("spec-gate review record は質問ダイアログの hook が実行する。エージェントは実行しない")
			}
		}
		if target, ok := shellWriteTarget(seg, fields, root); ok {
			return deny(target + " は保護しているのでシェル経由でも書き換えない")
		}
	}
	return allow()
}

var (
	// nestedShell は sh -c '…'・bash -lc "…"・eval '…' の中身
	nestedShell = regexp.MustCompile(`\b(?:ba|z|da|k)?sh\s+(?:-[a-zA-Z]+\s+)*-[a-zA-Z]*c\s+(?:'([^']*)'|"([^"]*)"|(\S+))|\beval\s+(?:'([^']*)'|"([^"]*)"|(\S+))`)
	// substitution は $( ( ) とバッククォート。区切りに置き換えて、中のコマンドも 1 つのコマンドとして見る
	substitution = regexp.MustCompile("\\$\\(|[()`]")
	separators   = regexp.MustCompile(`[;&|\n]+`)
)

// commandUnits はコマンドを、判定する単位（区切り・置き換え・入れ子のシェルの中身）に分ける。区切りすぎは拒否の側に倒れる。
func commandUnits(command string, depth int) []string {
	var out []string
	if depth < 4 {
		for _, m := range nestedShell.FindAllStringSubmatch(command, -1) {
			for _, g := range m[1:] {
				if g != "" {
					out = append(out, commandUnits(g, depth+1)...)
				}
			}
		}
	}
	return append(out, separators.Split(substitution.ReplaceAllString(command, ";"), -1)...)
}

// tokens は空白で区切り、各語の前後の引用符を外す。
func tokens(seg string) []string {
	f := strings.Fields(seg)
	for i := range f {
		f[i] = strings.Trim(f[i], `"'`)
	}
	return f
}

// wrapperFlagsWithValue は前置きのコマンドの、値を取るフラグ。
var wrapperFlagsWithValue = map[string]bool{"-u": true, "-g": true, "-n": true, "-s": true, "-k": true, "-C": true, "-D": true}

// wrappers は後ろのコマンドを実行する前置き。
var wrappers = map[string]bool{"sudo": true, "doas": true, "env": true, "time": true, "nohup": true, "exec": true, "command": true,
	"builtin": true, "nice": true, "xargs": true, "timeout": true, "stdbuf": true, "setsid": true}

// unwrap は先頭の環境変数の代入・前置きのコマンド（とそのフラグ）・mise exec -- を外す。
func unwrap(f []string) []string {
	for len(f) > 0 {
		head := filepath.Base(f[0])
		switch {
		case strings.Contains(f[0], "=") && !strings.HasPrefix(f[0], "-"):
			f = f[1:]
		case head == "mise" && len(f) > 1 && (f[1] == "exec" || f[1] == "x"):
			i := 2
			for i < len(f) && f[i] != "--" {
				i++
			}
			f = f[min(i+1, len(f)):]
		case wrappers[head]:
			f = f[1:]
			for len(f) > 0 && strings.HasPrefix(f[0], "-") {
				skip := 1
				if wrapperFlagsWithValue[f[0]] {
					skip = 2
				}
				f = f[min(skip, len(f)):]
			}
			if head == "timeout" && len(f) > 0 {
				f = f[1:] // 時間
			}
		default:
			return f
		}
	}
	return f
}

// goRunFlagsWithValue は go run の、値を取るフラグ。
var goRunFlagsWithValue = map[string]bool{"-tags": true, "-ldflags": true, "-gcflags": true, "-asmflags": true, "-o": true, "-C": true,
	"-mod": true, "-modfile": true, "-p": true, "-exec": true, "-toolexec": true, "-overlay": true, "-pgo": true, "-buildmode": true,
	"-compiler": true, "-gccgoflags": true, "-installsuffix": true, "-pkgdir": true, "-covermode": true, "-coverpkg": true}

// specGateSubcommand は spec-gate を呼ぶコマンド（前置きを外したもの）のサブコマンドを返す。
// spec-gate（パス付きも）と go run [フラグ] <パッケージ> を spec-gate とみなす。
func specGateSubcommand(fields []string) (string, bool) {
	switch {
	case len(fields) > 0 && slices.Contains([]string{"spec-gate", "spec-gate.exe", "spec-gate.real"}, filepath.Base(fields[0])):
		return subcommandAfter(fields[1:]), true
	case len(fields) > 1 && filepath.Base(fields[0]) == "go" && fields[1] == "run":
		i := 2
		for i < len(fields) && strings.HasPrefix(fields[i], "-") {
			name, _, hasValue := strings.Cut(fields[i], "=")
			if goRunFlagsWithValue[name] && !hasValue {
				i++
			}
			i++
		}
		if i < len(fields) {
			return subcommandAfter(fields[i+1:]), true
		}
	}
	return "", false
}

// subcommandAfter は spec-gate の後ろの語からサブコマンドを返す（-C / --root とその値、ほかのフラグを飛ばす）。
func subcommandAfter(f []string) string {
	for i := 0; i < len(f); {
		switch {
		case f[i] == "-C" || f[i] == "--root":
			i += 2
		case strings.HasPrefix(f[i], "-"):
			i++
		default:
			return f[i]
		}
	}
	return ""
}

var redirectTarget = regexp.MustCompile(`>{1,2}\s*["']?([^\s"';&|]+)`)

// writeCommands は、引数のパスを書き換えるコマンド（claims/** の保護と同じ顔ぶれに、ln・install・chmod を足したもの）。
var writeCommands = map[string]bool{"tee": true, "mv": true, "cp": true, "rm": true, "truncate": true, "ln": true, "install": true, "chmod": true}

// parentCommands は、引数のディレクトリの中まで消す・置き換える・書き込むコマンド。保護するパスの親（bin など）と、
// プロジェクトのルートやその上も拒否する（ルートごと動かせば入口が無くなり hook が素通りになる）。
var parentCommands = map[string]bool{"rm": true, "mv": true, "cp": true, "install": true, "chmod": true, "chown": true}

// coversRoot は path がプロジェクトのルートか、その上のディレクトリか。
func coversRoot(path, root string) bool {
	p := filepath.ToSlash(filepath.Clean(path))
	if root == "" || !filepath.IsAbs(path) {
		return p == "." || p == ".." || strings.HasPrefix(p, "../")
	}
	r := filepath.ToSlash(filepath.Clean(root))
	return r == p || strings.HasPrefix(r, strings.TrimSuffix(p, "/")+"/")
}

// gitSubcommand は git の前置きのフラグ（-C <dir>・-c <k=v>・--git-dir=…・--work-tree=… など）を飛ばしたサブコマンドの位置。
func gitSubcommand(fields []string) int {
	i := 1
	for i < len(fields) && strings.HasPrefix(fields[i], "-") {
		if fields[i] == "-C" || fields[i] == "-c" || fields[i] == "--git-dir" || fields[i] == "--work-tree" || fields[i] == "--namespace" {
			i++
		}
		i++
	}
	return i
}

// gobinTarget は GOBIN=<パス>（go install の出力先）。
var gobinTarget = regexp.MustCompile(`\bGOBIN=["']?([^\s"';&|]+)`)

// shellWriteTarget は、区切った 1 つのコマンドが保護するパスへ書き込むなら、そのパスを返す。
func shellWriteTarget(seg string, fields []string, root string) (string, bool) {
	for _, m := range redirectTarget.FindAllStringSubmatch(seg, -1) {
		if IsProtectedPath(m[1], root) {
			return m[1], true
		}
	}
	for _, m := range gobinTarget.FindAllStringSubmatch(seg, -1) {
		if coversProtected(m[1], root) {
			return m[1], true
		}
	}
	if len(fields) == 0 {
		return "", false
	}
	cmd := filepath.Base(fields[0])
	if cmd == "go" && len(fields) > 1 && fields[1] == "build" {
		return goBuildTarget(fields[2:], root)
	}
	// go install .（このモジュール）は GOBIN か ~/go/bin に spec-gate を置く。どちらも PATH に入りうる
	if cmd == "go" && len(fields) > 1 && fields[1] == "install" {
		for _, f := range fields[2:] {
			if f == "." || f == "./" {
				return "go install " + f, true
			}
		}
	}
	args := fields[1:]
	var gitRestore, treeish bool
	if cmd == "git" {
		i := gitSubcommand(fields)
		if i >= len(fields) {
			return "", false
		}
		sub, rest := fields[i], fields[i+1:]
		branch := len(rest) > 0 && (rest[0] == "-b" || rest[0] == "-B") // git checkout -b の引数はブランチ名
		staged := slices.Contains(rest, "--staged") && !slices.Contains(rest, "--worktree") && !slices.Contains(rest, "-W")
		gitRestore = (sub == "restore" && !staged) || (sub == "checkout" && !branch)
		// 版を指定して戻す（git checkout <rev> -- …・git restore --source=<rev> …）なら、ルート（.）も親とみなす
		if dash := slices.Index(rest, "--"); sub == "checkout" && dash > 0 {
			treeish = true
		}
		for _, r := range rest {
			treeish = treeish || r == "-s" || r == "--source" || strings.HasPrefix(r, "--source=")
		}
		args = rest
	}
	writes := writeCommands[cmd] || gitRestore ||
		(cmd == "sed" && len(fields) > 1 && strings.HasPrefix(fields[1], "-i"))
	if !writes {
		return "", false
	}
	// 消す・置き換える・戻すコマンドは、保護するパスの親ディレクトリも拒否する（git checkout HEAD~1 -- bin で古い入口に戻す、など）
	parent := parentCommands[cmd] || gitRestore
	whole := parentCommands[cmd] || treeish
	for _, f := range args {
		a := strings.Trim(f, `"'`)
		if _, v, ok := strings.Cut(a, "="); ok && strings.HasPrefix(a, "-") {
			a = v
		}
		if strings.HasPrefix(a, "-") && !strings.Contains(a, "/") {
			continue
		}
		if IsProtectedPath(a, root) || (parent && coversProtected(a, root)) || (whole && coversRoot(a, root)) ||
			(cmd != "rm" && outsideSpecGate(a, root)) {
			return a, true
		}
	}
	return "", false
}

// goBuildTarget は go build の出力先（-C <dir> と -o <パス>）が保護するパスか、その親（出力が <dir>/spec-gate になる）なら返す。
func goBuildTarget(args []string, root string) (string, bool) {
	var dir, out string
	for i := 0; i < len(args); i++ {
		a := strings.Trim(args[i], `"'`)
		name, v, has := strings.Cut(a, "=")
		if (name == "-o" || name == "-C") && !has && i+1 < len(args) {
			i++
			v = strings.Trim(args[i], `"'`)
		}
		switch name {
		case "-o":
			out = v
		case "-C":
			dir = v
		}
	}
	if out == "" {
		// -o が無ければ <dir>（-C）か今のディレクトリに spec-gate を書く
		if dir != "" && coversProtected(dir, root) {
			return dir, true
		}
		return "", false
	}
	target := out
	if dir != "" && !filepath.IsAbs(out) {
		target = filepath.Join(dir, out)
	}
	if coversProtected(target, root) || outsideSpecGate(target, root) {
		return out, true
	}
	return "", false
}

// outsideSpecGate は、プロジェクトの外の spec-gate という名前のファイルか。承認役は PATH 上の spec-gate を実行できるので、
// エージェントが組んだバイナリを PATH に置かせない（~/.local/bin/spec-gate など）。
func outsideSpecGate(path, root string) bool {
	if filepath.Base(path) != "spec-gate" {
		return false
	}
	_, inside := relToRoot(path, root)
	return !inside || (root == "" && filepath.IsAbs(path))
}
