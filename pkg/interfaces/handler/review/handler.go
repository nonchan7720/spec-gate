// Package review は承認待ち claim をブラウザで確認・承認するローカル Web 画面の HTTP ハンドラ。
// PdM やデザイナーが YAML を読まずに承認できるよう、claim を種類ごとの表や一覧で見せる。
package review

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/services"
)

//go:embed assets
var assets embed.FS

// Service は画面が使う操作（services.ReviewService が満たす）。
type Service interface {
	List(ctx context.Context) ([]services.ReviewItem, error)
	Approve(ctx context.Context, name, by string) (services.ApproveResult, error)
	Reject(name string) error
	// MarkAmbiguous は候補を「仕様が決まっていない」として claims/_ambiguous/ に理由つきで記録する
	MarkAmbiguous(name, reason string) error
}

type handler struct {
	svc Service
	by  string
}

// New は画面と API のハンドラを返す。by は承認者名として記録される。
func New(svc Service, by string) http.Handler {
	h := &handler{svc: svc, by: by}
	static, err := fs.Sub(assets, "assets")
	if err != nil {
		panic(err) // embed したディレクトリが無いのはビルドの誤り
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, static, "index.html")
	})
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServerFS(static)))
	mux.HandleFunc("GET /api/pending", h.pending)
	mux.HandleFunc("POST /api/approve", h.approve)
	mux.HandleFunc("POST /api/reject", h.reject)
	mux.HandleFunc("POST /api/ambiguous", h.ambiguous)
	return guard(mux)
}

// guard はローカル専用画面としての最低限の防御。
//   - Host がループバック名でなければ拒否（DNS rebinding で外部ページから読まれるのを防ぐ）
//   - 変更系は JSON のみ・同一 Origin のみ（他サイトのフォームや fetch から承認させない）
//   - CSP で外部リソースを禁じる
func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if !loopbackHost(r.Host) {
			writeError(w, http.StatusForbidden, "ローカル（127.0.0.1 / localhost）以外からのアクセスは受け付けない")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if o := r.Header.Get("Origin"); o != "" && o != "http://"+r.Host {
				writeError(w, http.StatusForbidden, "別の Origin からの操作は受け付けない")
				return
			}
			if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
				writeError(w, http.StatusUnsupportedMediaType, "Content-Type は application/json のみ")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func loopbackHost(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (h *handler) pending(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	views := make([]itemView, 0, len(items))
	for _, it := range items {
		views = append(views, toView(it))
	}
	writeJSON(w, http.StatusOK, map[string]any{"by": h.by, "items": views})
}

// actionResult は承認・却下 1 件の結果。
type actionResult struct {
	Name  string `json:"name"`
	ID    string `json:"id,omitempty"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func (h *handler) approve(w http.ResponseWriter, r *http.Request) {
	h.each(w, r, func(name string) actionResult {
		res, err := h.svc.Approve(r.Context(), name, h.by)
		if err != nil {
			return actionResult{Name: name, Error: err.Error()}
		}
		return actionResult{Name: name, ID: res.Claim.ID, OK: true}
	})
}

func (h *handler) reject(w http.ResponseWriter, r *http.Request) {
	h.each(w, r, func(name string) actionResult {
		if err := h.svc.Reject(name); err != nil {
			return actionResult{Name: name, Error: err.Error()}
		}
		return actionResult{Name: name, OK: true}
	})
}

// ambiguous は {"names": [...], "reason": "..."} を読み、候補を曖昧として理由つきで記録する。理由は必須。
func (h *handler) ambiguous(w http.ResponseWriter, r *http.Request) {
	h.eachWith(w, r, func(req actionRequest) string {
		if strings.TrimSpace(req.Reason) == "" {
			return "曖昧とする理由を入力してください"
		}
		return ""
	}, func(req actionRequest, name string) actionResult {
		if err := h.svc.MarkAmbiguous(name, req.Reason); err != nil {
			return actionResult{Name: name, Error: err.Error()}
		}
		return actionResult{Name: name, OK: true}
	})
}

// actionRequest は変更系 API の本文。
type actionRequest struct {
	Names []string `json:"names"`
	// Reason は曖昧の記録だけが使う
	Reason string `json:"reason"`
}

// each は {"names": [...]} を読み、1 件ずつ処理して結果を並べて返す。1 件の失敗で残りを止めない。
func (h *handler) each(w http.ResponseWriter, r *http.Request, fn func(name string) actionResult) {
	h.eachWith(w, r, nil, func(_ actionRequest, name string) actionResult { return fn(name) })
}

// eachWith は each に、本文全体の検査（validate が空でない文字列を返したら 400）を足したもの。
func (h *handler) eachWith(w http.ResponseWriter, r *http.Request, validate func(actionRequest) string, fn func(req actionRequest, name string) actionResult) {
	var req actionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "リクエストを読めない: "+err.Error())
		return
	}
	if validate != nil {
		if msg := validate(req); msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
	}
	if len(req.Names) == 0 {
		writeError(w, http.StatusBadRequest, "候補が選ばれていない")
		return
	}
	results := make([]actionResult, 0, len(req.Names))
	for _, name := range req.Names {
		results = append(results, fn(req, name))
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
