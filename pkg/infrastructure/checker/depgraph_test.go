package checker

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

type fakeRunner struct {
	out   string
	err   error
	calls []string
}

func (f *fakeRunner) Run(_ context.Context, command string) (string, error) {
	f.calls = append(f.calls, command)
	return f.out, f.err
}

func writeDepgraph(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	if body != "" {
		if err := os.WriteFile(filepath.Join(root, ".depgraph.yml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// 図: app -> domain, app -> infra, infra -> domain
var diagram = map[string]bool{"app -> domain": true, "app -> infra": true, "infra -> domain": true}

func archClaim() domain.Claim {
	return domain.Claim{
		ID: "ARCH-001", Kind: domain.KindStructure, Title: "層",
		Layers:    map[string]string{"app": "src/app", "domain": "src/domain", "infra": "src/infra"},
		Allowed:   "from-diagram",
		Forbidden: []string{"domain -> infra", "domain -> app"},
	}
}

const consistentYAML = `
layers:
  app: ["src/app/**"]
  domain: ["src/domain/**"]
  infra: ["src/infra/**"]
forbidden:
  domain: [app, infra]
  infra: [app]
`

// scanOK は depgraph scan が最後まで走ったときの出力（Findings なし）。
const scanOK = "project x\nscanned 39 files in 771ms\n"

func TestDepgraph_ConsistentAndScanCleanPasses(t *testing.T) {
	r := &fakeRunner{out: scanOK}
	d := NewDepgraph(writeDepgraph(t, consistentYAML), r, "depgraph scan")
	res := d.Check(context.Background(), archClaim(), domain.CheckOptions{DiagramEdges: diagram})
	if res.Status != domain.StatusPass {
		t.Fatalf("図と設定が一致し、全件スキャンで層違反が無ければ pass: %+v", res)
	}
	if len(r.calls) != 1 || r.calls[0] != "depgraph scan" {
		t.Fatalf("depgraph を 1 回実行すべき: %v", r.calls)
	}
}

func TestDepgraph_DefaultCommandIsFullScan(t *testing.T) {
	r := &fakeRunner{out: scanOK}
	d := NewDepgraph(writeDepgraph(t, consistentYAML), r, "")
	d.Check(context.Background(), archClaim(), domain.CheckOptions{DiagramEdges: diagram})
	// セッションで増えた分しか見ない gate ではなく、今のツリー全体を見る scan。CLAUDE.md は書き換えない
	if len(r.calls) != 1 || !strings.Contains(r.calls[0], "depgraph scan") || !strings.Contains(r.calls[0], "--claude-md ''") {
		t.Fatalf("既定は CLAUDE.md を書き換えない全件スキャン: %v", r.calls)
	}
}

func TestDepgraph_ExistingViolationFails(t *testing.T) {
	// セッション前からある違反でも fail にする（spec-gate は既存の仕様違反も通さない）
	out := scanOK + "findings (2):\n" +
		"  [error] layer-violation: src/domain -> src/infra (domain must not import infra)\n" +
		"  [warn] god-package: src/app is imported by 12 packages and imports 4\n"
	d := NewDepgraph(writeDepgraph(t, consistentYAML), &fakeRunner{out: out}, "depgraph scan")
	res := d.Check(context.Background(), archClaim(), domain.CheckOptions{DiagramEdges: diagram})
	if res.Status != domain.StatusFail || !strings.Contains(res.Detail, "src/domain -> src/infra") {
		t.Fatalf("claim の層どうしの layer-violation は fail: %+v", res)
	}
	if strings.Contains(res.Detail, "god-package") {
		t.Fatalf("layer-violation 以外は structure claim の対象外: %s", res.Detail)
	}
}

func TestDepgraph_ViolationOutsideClaimLayersIsIgnored(t *testing.T) {
	// .depgraph.yml には claim 以外の層（本体の core-* など）もある。それは別 claim / depgraph gate の担当
	out := scanOK + "findings (1):\n" +
		"  [error] layer-violation: pkg/domain -> pkg/services (core-domain must not import core-services)\n"
	d := NewDepgraph(writeDepgraph(t, consistentYAML), &fakeRunner{out: out}, "depgraph scan")
	res := d.Check(context.Background(), archClaim(), domain.CheckOptions{DiagramEdges: diagram})
	if res.Status != domain.StatusPass {
		t.Fatalf("claim の層を含まない layer-violation では fail にしない: %+v", res)
	}
}

func TestDepgraph_MissingConfigFails(t *testing.T) {
	r := &fakeRunner{}
	d := NewDepgraph(writeDepgraph(t, ""), r, "depgraph scan")
	res := d.Check(context.Background(), archClaim(), domain.CheckOptions{DiagramEdges: diagram})
	if res.Status != domain.StatusFail || !strings.Contains(res.Detail, ".depgraph.yml") {
		t.Fatalf(".depgraph.yml が無ければ fail: %+v", res)
	}
	if len(r.calls) != 0 {
		t.Fatalf("整合しないうちは depgraph を実行しない: %v", r.calls)
	}
}

func TestDepgraph_MissingLayerFails(t *testing.T) {
	yml := `
layers:
  app: ["src/app/**"]
  domain: ["src/domain/**"]
forbidden:
  domain: [app]
`
	d := NewDepgraph(writeDepgraph(t, yml), &fakeRunner{}, "depgraph scan")
	res := d.Check(context.Background(), archClaim(), domain.CheckOptions{DiagramEdges: diagram})
	if res.Status != domain.StatusFail || !strings.Contains(res.Detail, "infra") {
		t.Fatalf("claim の層が .depgraph.yml に無ければ fail: %+v", res)
	}
}

func TestDepgraph_LayerPathMismatchFails(t *testing.T) {
	yml := strings.Replace(consistentYAML, `infra: ["src/infra/**"]`, `infra: ["src/infrastructure/**"]`, 1)
	d := NewDepgraph(writeDepgraph(t, yml), &fakeRunner{}, "depgraph scan")
	res := d.Check(context.Background(), archClaim(), domain.CheckOptions{DiagramEdges: diagram})
	if res.Status != domain.StatusFail || !strings.Contains(res.Detail, "src/infra") {
		t.Fatalf("層のディレクトリが食い違えば fail: %+v", res)
	}
}

func TestDepgraph_ForbiddenMissingInDepgraphFails(t *testing.T) {
	// 図に無い infra -> app を depgraph が禁止していない = 違反を検出できない
	yml := strings.Replace(consistentYAML, "  infra: [app]\n", "", 1)
	d := NewDepgraph(writeDepgraph(t, yml), &fakeRunner{}, "depgraph scan")
	res := d.Check(context.Background(), archClaim(), domain.CheckOptions{DiagramEdges: diagram})
	if res.Status != domain.StatusFail || !strings.Contains(res.Detail, "infra -> app") {
		t.Fatalf("図で許可されない依存を depgraph が禁止していなければ fail: %+v", res)
	}
}

func TestDepgraph_DepgraphForbidsAllowedEdgeFails(t *testing.T) {
	// 図で許可された app -> infra を depgraph が禁止している = 仕様と矛盾
	yml := strings.Replace(consistentYAML, "  infra: [app]\n", "  infra: [app]\n  app: [infra]\n", 1)
	d := NewDepgraph(writeDepgraph(t, yml), &fakeRunner{}, "depgraph scan")
	res := d.Check(context.Background(), archClaim(), domain.CheckOptions{DiagramEdges: diagram})
	if res.Status != domain.StatusFail || !strings.Contains(res.Detail, "app -> infra") {
		t.Fatalf("図で許可された依存を depgraph が禁止していれば fail: %+v", res)
	}
}

func TestDepgraph_ClaimForbiddenOverridesDiagram(t *testing.T) {
	// 図に app -> infra があっても claim が明示的に禁止すれば depgraph も禁止しているべき
	c := archClaim()
	c.Forbidden = append(c.Forbidden, "app -> infra")
	d := NewDepgraph(writeDepgraph(t, consistentYAML), &fakeRunner{}, "depgraph scan")
	res := d.Check(context.Background(), c, domain.CheckOptions{DiagramEdges: diagram})
	if res.Status != domain.StatusFail || !strings.Contains(res.Detail, "app -> infra") {
		t.Fatalf("claim の forbidden は図より優先: %+v", res)
	}
}

func TestDepgraph_WithoutDiagramOnlyClaimForbidden(t *testing.T) {
	c := archClaim()
	c.Allowed = ""
	yml := `
layers:
  app: ["src/app/**"]
  domain: ["src/domain/**"]
  infra: ["src/infra/**"]
forbidden:
  domain: [infra, app]
`
	d := NewDepgraph(writeDepgraph(t, yml), &fakeRunner{out: scanOK}, "depgraph scan")
	res := d.Check(context.Background(), c, domain.CheckOptions{})
	if res.Status != domain.StatusPass {
		t.Fatalf("from-diagram でなければ claim の forbidden とだけ照合する: %+v", res)
	}
}

func TestDepgraph_FromDiagramWithoutEdgesFails(t *testing.T) {
	d := NewDepgraph(writeDepgraph(t, consistentYAML), &fakeRunner{}, "depgraph scan")
	res := d.Check(context.Background(), archClaim(), domain.CheckOptions{})
	if res.Status != domain.StatusFail {
		t.Fatalf("図のエッジが取れなければ fail: %+v", res)
	}
}

func TestDepgraph_ScanErrorIsUnverified(t *testing.T) {
	// scan が非0で終わるのは Findings ではなく環境の問題（Neo4j に届かない等）
	r := &fakeRunner{out: "connect bolt://localhost:17687: connection refused", err: errors.New("exit status 1")}
	d := NewDepgraph(writeDepgraph(t, consistentYAML), r, "depgraph scan")
	res := d.Check(context.Background(), archClaim(), domain.CheckOptions{DiagramEdges: diagram})
	if res.Status != domain.StatusUnverified || !strings.Contains(res.Detail, "connection refused") {
		t.Fatalf("depgraph が判定できなければ unverified（理由つき）: %+v", res)
	}
}

func TestDepgraph_IncompleteScanIsUnverified(t *testing.T) {
	// exit 0 でもスキャン完了の表示が無ければ、判定したことにしない
	r := &fakeRunner{out: "project x\n"}
	d := NewDepgraph(writeDepgraph(t, consistentYAML), r, "depgraph scan")
	res := d.Check(context.Background(), archClaim(), domain.CheckOptions{DiagramEdges: diagram})
	if res.Status != domain.StatusUnverified {
		t.Fatalf("スキャン完了が確認できなければ unverified: %+v", res)
	}
}

func TestDepgraph_NotInstalledIsUnverified(t *testing.T) {
	// sh が返す 127（command not found）を本物の ExitError で再現する
	err := exec.Command("sh", "-c", "exit 127").Run()
	r := &fakeRunner{out: "sh: depgraph: command not found", err: err}
	d := NewDepgraph(writeDepgraph(t, consistentYAML), r, "depgraph scan")
	res := d.Check(context.Background(), archClaim(), domain.CheckOptions{DiagramEdges: diagram})
	if res.Status != domain.StatusUnverified || !strings.Contains(res.Detail, "depgraph") {
		t.Fatalf("depgraph が無ければ unverified: %+v", res)
	}
}
