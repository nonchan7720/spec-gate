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

func TestDepgraph_ConsistentAndGatePasses(t *testing.T) {
	r := &fakeRunner{}
	d := NewDepgraph(writeDepgraph(t, consistentYAML), r, "depgraph gate")
	res := d.Check(context.Background(), archClaim(), domain.CheckOptions{DiagramEdges: diagram})
	if res.Status != domain.StatusPass {
		t.Fatalf("図と設定が一致し gate も通れば pass: %+v", res)
	}
	if len(r.calls) != 1 || r.calls[0] != "depgraph gate" {
		t.Fatalf("depgraph gate を 1 回実行すべき: %v", r.calls)
	}
}

func TestDepgraph_MissingConfigFails(t *testing.T) {
	r := &fakeRunner{}
	d := NewDepgraph(writeDepgraph(t, ""), r, "depgraph gate")
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
	d := NewDepgraph(writeDepgraph(t, yml), &fakeRunner{}, "depgraph gate")
	res := d.Check(context.Background(), archClaim(), domain.CheckOptions{DiagramEdges: diagram})
	if res.Status != domain.StatusFail || !strings.Contains(res.Detail, "infra") {
		t.Fatalf("claim の層が .depgraph.yml に無ければ fail: %+v", res)
	}
}

func TestDepgraph_LayerPathMismatchFails(t *testing.T) {
	yml := strings.Replace(consistentYAML, `infra: ["src/infra/**"]`, `infra: ["src/infrastructure/**"]`, 1)
	d := NewDepgraph(writeDepgraph(t, yml), &fakeRunner{}, "depgraph gate")
	res := d.Check(context.Background(), archClaim(), domain.CheckOptions{DiagramEdges: diagram})
	if res.Status != domain.StatusFail || !strings.Contains(res.Detail, "src/infra") {
		t.Fatalf("層のディレクトリが食い違えば fail: %+v", res)
	}
}

func TestDepgraph_ForbiddenMissingInDepgraphFails(t *testing.T) {
	// 図に無い infra -> app を depgraph が禁止していない = 違反を検出できない
	yml := strings.Replace(consistentYAML, "  infra: [app]\n", "", 1)
	d := NewDepgraph(writeDepgraph(t, yml), &fakeRunner{}, "depgraph gate")
	res := d.Check(context.Background(), archClaim(), domain.CheckOptions{DiagramEdges: diagram})
	if res.Status != domain.StatusFail || !strings.Contains(res.Detail, "infra -> app") {
		t.Fatalf("図で許可されない依存を depgraph が禁止していなければ fail: %+v", res)
	}
}

func TestDepgraph_DepgraphForbidsAllowedEdgeFails(t *testing.T) {
	// 図で許可された app -> infra を depgraph が禁止している = 仕様と矛盾
	yml := strings.Replace(consistentYAML, "  infra: [app]\n", "  infra: [app]\n  app: [infra]\n", 1)
	d := NewDepgraph(writeDepgraph(t, yml), &fakeRunner{}, "depgraph gate")
	res := d.Check(context.Background(), archClaim(), domain.CheckOptions{DiagramEdges: diagram})
	if res.Status != domain.StatusFail || !strings.Contains(res.Detail, "app -> infra") {
		t.Fatalf("図で許可された依存を depgraph が禁止していれば fail: %+v", res)
	}
}

func TestDepgraph_ClaimForbiddenOverridesDiagram(t *testing.T) {
	// 図に app -> infra があっても claim が明示的に禁止すれば depgraph も禁止しているべき
	c := archClaim()
	c.Forbidden = append(c.Forbidden, "app -> infra")
	d := NewDepgraph(writeDepgraph(t, consistentYAML), &fakeRunner{}, "depgraph gate")
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
	d := NewDepgraph(writeDepgraph(t, yml), &fakeRunner{}, "depgraph gate")
	res := d.Check(context.Background(), c, domain.CheckOptions{})
	if res.Status != domain.StatusPass {
		t.Fatalf("from-diagram でなければ claim の forbidden とだけ照合する: %+v", res)
	}
}

func TestDepgraph_FromDiagramWithoutEdgesFails(t *testing.T) {
	d := NewDepgraph(writeDepgraph(t, consistentYAML), &fakeRunner{}, "depgraph gate")
	res := d.Check(context.Background(), archClaim(), domain.CheckOptions{})
	if res.Status != domain.StatusFail {
		t.Fatalf("図のエッジが取れなければ fail: %+v", res)
	}
}

func TestDepgraph_GateFailureFails(t *testing.T) {
	r := &fakeRunner{out: "layer-violation: src/domain -> src/infra", err: errors.New("exit status 1")}
	d := NewDepgraph(writeDepgraph(t, consistentYAML), r, "depgraph gate")
	res := d.Check(context.Background(), archClaim(), domain.CheckOptions{DiagramEdges: diagram})
	if res.Status != domain.StatusFail || !strings.Contains(res.Detail, "layer-violation") {
		t.Fatalf("depgraph gate が非0なら fail（出力つき）: %+v", res)
	}
}

func TestDepgraph_GateSkippedIsUnverified(t *testing.T) {
	r := &fakeRunner{out: "depgraph gate: no running store\ndepgraph gate: skipped"}
	d := NewDepgraph(writeDepgraph(t, consistentYAML), r, "depgraph gate")
	res := d.Check(context.Background(), archClaim(), domain.CheckOptions{DiagramEdges: diagram})
	if res.Status != domain.StatusUnverified || !strings.Contains(res.Detail, "no running store") {
		t.Fatalf("depgraph が判定できずに通したなら unverified: %+v", res)
	}
}

func TestDepgraph_NotInstalledIsUnverified(t *testing.T) {
	// sh が返す 127（command not found）を本物の ExitError で再現する
	err := exec.Command("sh", "-c", "exit 127").Run()
	r := &fakeRunner{out: "sh: depgraph: command not found", err: err}
	d := NewDepgraph(writeDepgraph(t, consistentYAML), r, "depgraph gate")
	res := d.Check(context.Background(), archClaim(), domain.CheckOptions{DiagramEdges: diagram})
	if res.Status != domain.StatusUnverified || !strings.Contains(res.Detail, "depgraph") {
		t.Fatalf("depgraph が無ければ unverified: %+v", res)
	}
}
