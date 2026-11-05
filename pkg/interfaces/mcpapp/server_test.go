package mcpapp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nonchan7720/spec-gate/pkg/services"
)

type fakeSvc struct {
	items   []services.AppItem
	decided []services.AppDecision
	bulk    []services.AppDecision
	by      string
}

func (f *fakeSvc) AppItems(context.Context) ([]services.AppItem, error) { return f.items, nil }
func (f *fakeSvc) AppDecide(_ context.Context, d services.AppDecision, by string) error {
	f.decided = append(f.decided, d)
	f.by = by
	return nil
}
func (f *fakeSvc) AppBulkApprove(_ context.Context, ds []services.AppDecision, _ string) []services.BulkOutcome {
	f.bulk = ds
	var out []services.BulkOutcome
	for _, d := range ds {
		out = append(out, services.BulkOutcome{Name: d.Name, ID: "X"})
	}
	return out
}

func connect(t *testing.T, svc Service) *mcp.ClientSession {
	t.Helper()
	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	ss, err := NewServer(svc, "nonchan", "test").Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func visibility(tool *mcp.Tool) []string {
	ui, _ := tool.Meta["ui"].(map[string]any)
	var out []string
	if v, ok := ui["visibility"].([]any); ok {
		for _, x := range v {
			out = append(out, x.(string))
		}
	}
	return out
}

func TestServer_ToolsAndResource(t *testing.T) {
	cs := connect(t, &fakeSvc{})
	ctx := context.Background()
	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]string{}
	for _, tool := range tools.Tools {
		got[tool.Name] = visibility(tool)
		ui, _ := tool.Meta["ui"].(map[string]any)
		if ui["resourceUri"] != ResourceURI {
			t.Errorf("%s: _meta.ui.resourceUri が %s を指す: %v", tool.Name, ResourceURI, tool.Meta)
		}
	}
	want := map[string]string{ToolReview: "model,app", ToolList: "app", ToolDecide: "app", ToolBulk: "app"}
	for name, v := range want {
		if strings.Join(got[name], ",") != v {
			t.Errorf("%s の visibility は %s（判断を反映するツールはエージェントに見せない）: %v", name, v, got[name])
		}
	}

	res, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: ResourceURI})
	if err != nil {
		t.Fatal(err)
	}
	if c := res.Contents[0]; c.MIMEType != "text/html;profile=mcp-app" || !strings.Contains(c.Text, "ui/initialize") || !strings.Contains(c.Text, "review_decide") {
		t.Fatalf("UI は MCP Apps の HTML: %s", c.MIMEType)
	}
}

func TestServer_ReviewReturnsSummaryAndItems(t *testing.T) {
	svc := &fakeSvc{items: []services.AppItem{
		{Name: "A.yaml", ID: "A", Title: "円未満は切り捨て", Kind: "example", Confidence: "3/3", DryRun: "pass", Eligible: true},
		{Name: "B.yaml", ID: "B", Title: "締めは月末", Kind: "manual", Confidence: "1/3"},
	}}
	cs := connect(t, svc)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: ToolReview})
	if err != nil {
		t.Fatal(err)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "承認待ちの候補 2 件（まとめ承認の対象 1 件") || !strings.Contains(text, "A 円未満は切り捨て") {
		t.Fatalf("画面を出せないホスト向けの要約: %s", text)
	}
	b, _ := json.Marshal(res.StructuredContent)
	var ov map[string]json.RawMessage
	if err := json.Unmarshal(b, &ov); err != nil {
		t.Fatal(err)
	}
	if _, has := ov["items"]; has {
		t.Fatalf("review の structuredContent に候補の一覧を入れない（画面は review_list で読む）: %s", b)
	}
	var o Overview
	if err := json.Unmarshal(b, &o); err != nil || o.Counts["total"] != 2 || o.Counts["eligible"] != 1 || o.Counts["kind:manual"] != 1 {
		t.Fatalf("structuredContent に件数の内訳: %s", b)
	}
}

func TestServer_DecideValidatesAndPassesThrough(t *testing.T) {
	svc := &fakeSvc{}
	cs := connect(t, svc)
	ctx := context.Background()
	call := func(args map[string]any) DecideOut {
		t.Helper()
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: ToolDecide, Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(res.StructuredContent)
		var out DecideOut
		_ = json.Unmarshal(b, &out)
		return out
	}
	if out := call(map[string]any{"name": "A.yaml", "verdict": "delete", "content_sha256": "x"}); out.OK || out.Error == "" {
		t.Fatalf("知らない verdict は反映しない: %+v", out)
	}
	if out := call(map[string]any{"name": "A.yaml", "verdict": "ambiguous", "content_sha256": "x"}); out.OK || !strings.Contains(out.Error, "理由") {
		t.Fatalf("曖昧は理由が必須: %+v", out)
	}
	if out := call(map[string]any{"name": "A.yaml", "verdict": "approve", "content_sha256": "abc"}); !out.OK {
		t.Fatalf("承認を渡す: %+v", out)
	}
	if len(svc.decided) != 1 || svc.decided[0].ContentSHA256 != "abc" || svc.by != "nonchan" {
		t.Fatalf("見せた内容の sha256 と承認者を渡す: %+v %q", svc.decided, svc.by)
	}

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: ToolBulk, Arguments: map[string]any{"items": []map[string]any{{"name": "A.yaml", "content_sha256": "abc"}}}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(res.StructuredContent)
	if !strings.Contains(string(b), `"name":"A.yaml"`) || len(svc.bulk) != 1 {
		t.Fatalf("まとめ承認の結果を名前つきで返す: %s", b)
	}
}
