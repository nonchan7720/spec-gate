package review

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/services"
)

type fakeService struct {
	items    []services.ReviewItem
	listErr  error
	failures map[string]string // name → 承認時のエラー
	approved []string          // "name by"
	rejected []string
}

func (f *fakeService) List(context.Context) ([]services.ReviewItem, error) {
	return f.items, f.listErr
}

func (f *fakeService) Approve(_ context.Context, name, by string) (services.ApproveResult, error) {
	if msg, ng := f.failures[name]; ng {
		return services.ApproveResult{}, errors.New(msg)
	}
	f.approved = append(f.approved, name+" "+by)
	return services.ApproveResult{Claim: domain.Claim{ID: strings.TrimSuffix(name, ".yaml"), File: "/abs/claims/spec/" + name}}, nil
}

func (f *fakeService) Reject(name string) error {
	if msg, ng := f.failures[name]; ng {
		return errors.New(msg)
	}
	f.rejected = append(f.rejected, name)
	return nil
}

func exampleItem() services.ReviewItem {
	return services.ReviewItem{
		Name: "ROUND-9.yaml", Path: "claims/_pending/ROUND-9.yaml", Confidence: "2/3",
		Claim: domain.Claim{
			ID: "ROUND-9", Kind: domain.KindExample, Title: "円未満は切り捨て",
			Source:     domain.Source{Type: domain.SourceMarkdown, Ref: "docs/spec/rounding.md#端数処理"},
			Binding:    "go run ./tools/call -fn roundYen",
			Examples:   []domain.Example{{In: 1234.56, Out: 1234}, {In: []any{1, 2}, Out: 3, Note: "引数 2 つ"}, {In: "abc", Out: nil}},
			Properties: []string{"isInteger(output)"},
		},
		Area: "spec",
		Spec: domain.Snapshot{Hash: "h", Excerpt: "## 端数処理\n円未満は切り捨てる", Location: "docs/spec/rounding.md:3"},
	}
}

func structureItem() services.ReviewItem {
	return services.ReviewItem{
		Name: "ARCH-9.yaml", Confidence: "3/3",
		Claim: domain.Claim{
			ID: "ARCH-9", Kind: domain.KindStructure, Title: "層",
			Source:    domain.Source{Type: domain.SourceMermaid, Ref: "docs/arch/layers.mmd"},
			Layers:    map[string]string{"infra": "src/infra", "app": "src/app"},
			Allowed:   "from-diagram",
			Forbidden: []string{"domain -> infra"},
		},
		Area:      "arch",
		SpecError: "仕様を読めない: ファイルが無い",
	}
}

func do(t *testing.T, h http.Handler, method, target, body string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Host = "127.0.0.1:7790"
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

var jsonHdr = map[string]string{"Content-Type": "application/json"}

func TestIndex_ServesEmbeddedPage(t *testing.T) {
	h := New(&fakeService{}, "alice")
	rec := do(t, h, http.MethodGet, "/", "", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "承認") {
		t.Fatalf("トップページを返す: %d %s", rec.Code, rec.Body.String())
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
		t.Fatalf("外部リソースを CSP で禁じる: %q", csp)
	}
	for _, p := range []string{"/assets/app.js", "/assets/style.css"} {
		if rec := do(t, h, http.MethodGet, p, "", nil); rec.Code != http.StatusOK {
			t.Fatalf("%s を返す: %d", p, rec.Code)
		}
	}
	if rec := do(t, h, http.MethodGet, "/nope", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("知らないパスは 404: %d", rec.Code)
	}
}

func TestAssets_NoExternalResources(t *testing.T) {
	ext := regexp.MustCompile(`(?i)(https?:)?//[a-z0-9.-]+\.[a-z]{2,}`)
	err := fs.WalkDir(assets, "assets", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(assets, p)
		if err != nil {
			return err
		}
		if m := ext.Find(b); m != nil {
			t.Errorf("%s が外部 URL を参照している（CDN・外部フォント禁止）: %s", p, m)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPending_PlainLanguageView(t *testing.T) {
	broken := services.ReviewItem{Name: "B.yaml", Error: "title がない"}
	h := New(&fakeService{items: []services.ReviewItem{exampleItem(), structureItem(), broken}}, "alice")
	rec := do(t, h, http.MethodGet, "/api/pending", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var got struct {
		By    string     `json:"by"`
		Items []itemView `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.By != "alice" || len(got.Items) != 3 {
		t.Fatalf("承認者名と全件: %+v", got)
	}
	ex := got.Items[0]
	if ex.ID != "ROUND-9" || ex.Kind != "example" || ex.Confidence != "2/3" || ex.Area != "spec" || ex.Source.Ref != "docs/spec/rounding.md#端数処理" {
		t.Fatalf("基本情報: %+v", ex)
	}
	if ex.Spec.Excerpt != "## 端数処理\n円未満は切り捨てる" || ex.Spec.Location != "docs/spec/rounding.md:3" {
		t.Fatalf("仕様の抜粋: %+v", ex.Spec)
	}
	wantEx := []exampleView{{In: "1234.56", Out: "1234"}, {In: "1, 2", Out: "3", Note: "引数 2 つ"}, {In: `"abc"`, Out: "なし"}}
	if len(ex.Examples) != len(wantEx) {
		t.Fatalf("examples: %+v", ex.Examples)
	}
	for i, w := range wantEx {
		if ex.Examples[i] != w {
			t.Errorf("examples[%d] = %+v, want %+v", i, ex.Examples[i], w)
		}
	}
	if len(ex.Properties) != 1 || ex.Properties[0] != "isInteger(output)" {
		t.Fatalf("properties: %+v", ex.Properties)
	}

	st := got.Items[1]
	if len(st.Layers) != 2 || st.Layers[0] != (layerView{Name: "app", Dir: "src/app"}) || !st.FromDiagram {
		t.Fatalf("層は名前順・図のエッジのみ許可: %+v", st)
	}
	if len(st.Forbidden) != 1 || st.Forbidden[0] != (edgeView{From: "domain", To: "infra"}) {
		t.Fatalf("禁止エッジ: %+v", st.Forbidden)
	}
	if st.Spec.Error != "仕様を読めない: ファイルが無い" {
		t.Fatalf("仕様を読めない理由: %+v", st.Spec)
	}
	if got.Items[2].Error != "title がない" {
		t.Fatalf("壊れた候補のエラー: %+v", got.Items[2])
	}
	if strings.Contains(rec.Body.String(), "binding:") || strings.Contains(rec.Body.String(), "source:") {
		t.Fatalf("YAML をそのまま出さない: %s", rec.Body.String())
	}
}

func TestPending_ListError(t *testing.T) {
	h := New(&fakeService{listErr: errors.New("読めない")}, "alice")
	rec := do(t, h, http.MethodGet, "/api/pending", "", nil)
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "読めない") {
		t.Fatalf("一覧の失敗は 500 と理由: %d %s", rec.Code, rec.Body.String())
	}
}

type resultsBody struct {
	Results []actionResult `json:"results"`
}

func TestApprove_EachNameWithResult(t *testing.T) {
	svc := &fakeService{failures: map[string]string{"B.yaml": "lint に失敗。承認できない"}}
	h := New(svc, "alice")
	rec := do(t, h, http.MethodPost, "/api/approve", `{"names":["A.yaml","B.yaml"]}`, jsonHdr)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var got resultsBody
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 2 || !got.Results[0].OK || got.Results[1].OK || got.Results[1].Error != "lint に失敗。承認できない" {
		t.Fatalf("1 件ずつ結果を返す（失敗は理由つき）: %+v", got)
	}
	if len(svc.approved) != 1 || svc.approved[0] != "A.yaml alice" {
		t.Fatalf("--by の承認者で承認する: %+v", svc.approved)
	}
}

func TestReject_EachName(t *testing.T) {
	svc := &fakeService{}
	h := New(svc, "alice")
	rec := do(t, h, http.MethodPost, "/api/reject", `{"names":["A.yaml"]}`, jsonHdr)
	var got resultsBody
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || len(got.Results) != 1 || !got.Results[0].OK || len(svc.rejected) != 1 {
		t.Fatalf("却下: %d %+v %+v", rec.Code, got, svc.rejected)
	}
}

func TestGuards(t *testing.T) {
	h := New(&fakeService{}, "alice")
	cases := []struct {
		name   string
		method string
		target string
		body   string
		hdr    map[string]string
		host   string
		want   int
	}{
		{"ローカル以外の Host（DNS rebinding）", http.MethodGet, "/api/pending", "", nil, "evil.example:7790", http.StatusForbidden},
		{"JSON 以外の POST（フォーム送信の CSRF）", http.MethodPost, "/api/approve", `names=A.yaml`, map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, "", http.StatusUnsupportedMediaType},
		{"別 Origin からの POST", http.MethodPost, "/api/approve", `{"names":["A.yaml"]}`, map[string]string{"Content-Type": "application/json", "Origin": "http://evil.example"}, "", http.StatusForbidden},
		{"壊れた JSON", http.MethodPost, "/api/approve", `{`, jsonHdr, "", http.StatusBadRequest},
		{"空の names", http.MethodPost, "/api/approve", `{"names":[]}`, jsonHdr, "", http.StatusBadRequest},
		{"GET で承認はできない", http.MethodGet, "/api/approve", "", nil, "", http.StatusMethodNotAllowed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.target, strings.NewReader(tc.body))
			req.Host = "127.0.0.1:7790"
			if tc.host != "" {
				req.Host = tc.host
			}
			for k, v := range tc.hdr {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
	ok := httptest.NewRequest(http.MethodPost, "/api/reject", strings.NewReader(`{"names":["A.yaml"]}`))
	ok.Host = "localhost:7790"
	ok.Header.Set("Content-Type", "application/json; charset=utf-8")
	ok.Header.Set("Origin", "http://localhost:7790")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, ok)
	if rec.Code != http.StatusOK {
		t.Fatalf("同一 Origin の localhost は通す: %d %s", rec.Code, rec.Body.String())
	}
}
