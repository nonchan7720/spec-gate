// Package mcpapp は MCP Apps の承認 UI（docs/spec/harness.md「MCP Apps の承認 UI」）。
// リモートセッション（claude.ai/code・スマホの Claude アプリ）から、承認待ち候補を読んでタップで判断できるようにする。
// 判断を反映するツールは UI からだけ呼べる（_meta.ui.visibility: ["app"]）。エージェントには見えず、呼べない。
package mcpapp

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/services"
)

//go:embed assets/review.html
var reviewHTML string

// 名前と URI。
const (
	ResourceURI = "ui://spec-gate/review"
	MIMEType    = "text/html;profile=mcp-app"

	ToolReview     = "review"
	ToolList       = "review_list"
	ToolDecide     = "review_decide"
	ToolBulk       = "review_bulk_approve"
	summaryListMax = 10
)

// Service は UI が使う操作（services.RemoteReviewService が満たす）。
type Service interface {
	AppItems(ctx context.Context) ([]services.AppItem, error)
	AppDecide(ctx context.Context, d services.AppDecision, by string) error
	AppBulkApprove(ctx context.Context, ds []services.AppDecision, by string) []services.BulkOutcome
}

// Items は候補の一覧（ツールの structuredContent）。
type Items struct {
	Items []services.AppItem `json:"items"`
	// Counts は試し実行の結果・kind ごとの件数など、一覧の上に出す内訳
	Counts map[string]int `json:"counts"`
}

// DecideIn は review_decide の入力。
type DecideIn struct {
	Name          string `json:"name" jsonschema:"候補のファイル名"`
	Verdict       string `json:"verdict" jsonschema:"approve / reject / ambiguous"`
	Reason        string `json:"reason,omitempty" jsonschema:"ambiguous の理由"`
	ContentSHA256 string `json:"content_sha256" jsonschema:"画面に出した内容の sha256"`
}

// DecideOut は review_decide の結果。
type DecideOut struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// BulkItem はまとめ承認する候補 1 件。
type BulkItem struct {
	Name          string `json:"name" jsonschema:"候補のファイル名"`
	ContentSHA256 string `json:"content_sha256" jsonschema:"画面に出した内容の sha256"`
}

// BulkIn は review_bulk_approve の入力。
type BulkIn struct {
	Items []BulkItem `json:"items" jsonschema:"承認する候補"`
}

// BulkOut は review_bulk_approve の結果。
type BulkOut struct {
	Results []services.BulkOutcome `json:"results"`
}

func uiMeta(visibility ...string) mcp.Meta {
	return mcp.Meta{"ui": map[string]any{"resourceUri": ResourceURI, "visibility": visibility}}
}

// NewServer は承認 UI の MCP サーバを作る。by は承認者として記録される。
func NewServer(svc Service, by, version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "spec-gate", Version: version}, nil)

	s.AddResource(&mcp.Resource{
		URI: ResourceURI, Name: "spec-gate-review", Title: "spec-gate 承認",
		Description: "承認待ちの claim 候補を読んで、承認・却下・曖昧・まとめ承認をタップで決める画面",
		MIMEType:    MIMEType,
		Meta:        mcp.Meta{"ui": map[string]any{"prefersBorder": true}},
	}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: ResourceURI, MIMEType: MIMEType, Text: reviewHTML}}}, nil
	})

	list := func(ctx context.Context) (Items, error) {
		items, err := svc.AppItems(ctx)
		if err != nil {
			return Items{}, err
		}
		return Items{Items: items, Counts: counts(items)}, nil
	}

	// エージェントが呼んで画面を開く。判断はしない（一覧の要約だけを返す）
	mcp.AddTool(s, &mcp.Tool{
		Name:  ToolReview,
		Title: "spec-gate の承認画面を開く",
		Description: "claims/_pending/ の承認待ち候補を、利用者がタップで承認・却下・曖昧・まとめ承認できる画面で開く。" +
			"判断は利用者が画面で行い、エージェントは判断しない。",
		Meta:        uiMeta("model", "app"),
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, Items, error) {
		it, err := list(ctx)
		if err != nil {
			return nil, Items{}, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: Summary(it)}}}, it, nil
	})

	// 以下は画面からだけ呼べる（visibility: app）。エージェントには見えない
	mcp.AddTool(s, &mcp.Tool{
		Name: ToolList, Description: "承認待ち候補の一覧（画面用）", Meta: uiMeta("app"),
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, Items, error) {
		it, err := list(ctx)
		return nil, it, err
	})
	mcp.AddTool(s, &mcp.Tool{
		Name: ToolDecide, Description: "候補 1 件への判断を反映する（画面用。利用者が押したときだけ）", Meta: uiMeta("app"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in DecideIn) (*mcp.CallToolResult, DecideOut, error) {
		v := domain.RemoteVerdict(in.Verdict)
		switch v {
		case domain.VerdictApprove, domain.VerdictReject, domain.VerdictAmbiguous:
		default:
			return nil, DecideOut{Error: fmt.Sprintf("verdict は approve / reject / ambiguous: %q", in.Verdict)}, nil
		}
		if v == domain.VerdictAmbiguous && strings.TrimSpace(in.Reason) == "" {
			return nil, DecideOut{Error: "曖昧とする理由を入力してください"}, nil
		}
		if err := svc.AppDecide(ctx, services.AppDecision{Name: in.Name, Verdict: v, Reason: in.Reason, ContentSHA256: in.ContentSHA256}, by); err != nil {
			return nil, DecideOut{Error: err.Error()}, nil
		}
		return nil, DecideOut{OK: true}, nil
	})
	mcp.AddTool(s, &mcp.Tool{
		Name: ToolBulk, Description: "候補をまとめて承認する（画面用。利用者が押したときだけ）", Meta: uiMeta("app"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in BulkIn) (*mcp.CallToolResult, BulkOut, error) {
		ds := make([]services.AppDecision, 0, len(in.Items))
		for _, it := range in.Items {
			ds = append(ds, services.AppDecision{Name: it.Name, Verdict: domain.VerdictApprove, ContentSHA256: it.ContentSHA256})
		}
		return nil, BulkOut{Results: svc.AppBulkApprove(ctx, ds, by)}, nil
	})
	return s
}

func counts(items []services.AppItem) map[string]int {
	c := map[string]int{"total": len(items)}
	for _, it := range items {
		c["kind:"+it.Kind]++
		if it.DryRun != "" {
			c["dry_run:"+string(domain.DryRunStateOf(it.DryRun))]++
		}
		if it.Eligible {
			c["eligible"]++
		}
		if it.Error != "" {
			c["broken"]++
		}
	}
	return c
}

// Summary は画面を表示できないホスト向けの要約（エージェントにも見える）。
func Summary(it Items) string {
	var b strings.Builder
	c := it.Counts
	fmt.Fprintf(&b, "承認待ちの候補 %d 件（まとめ承認の対象 %d 件、読めない候補 %d 件）。判断は利用者が承認画面で行う。\n",
		c["total"], c["eligible"], c["broken"])
	fmt.Fprintf(&b, "kind: example %d / structure %d / ui %d / manual %d。試し実行: pass %d / fail %d / error %d\n",
		c["kind:example"], c["kind:structure"], c["kind:ui"], c["kind:manual"], c["dry_run:pass"], c["dry_run:fail"], c["dry_run:error"])
	for i, x := range it.Items {
		if i == summaryListMax {
			fmt.Fprintf(&b, "…ほか %d 件\n", len(it.Items)-i)
			break
		}
		fmt.Fprintf(&b, "- %s %s（%s, %s, %s）\n", x.ID, x.Title, x.Kind, x.Confidence, orDash(x.DryRun))
	}
	return b.String()
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
