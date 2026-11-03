package review

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"strings"
	"testing"
)

func (f *fakeService) MarkAmbiguous(name, reason string) error {
	if msg, ng := f.failures[name]; ng {
		return errors.New(msg)
	}
	f.ambiguous = append(f.ambiguous, name+": "+reason)
	return nil
}

func TestAmbiguous_RecordsWithReason(t *testing.T) {
	svc := &fakeService{failures: map[string]string{"B.yaml": "候補 B.yaml がない"}}
	h := New(svc, "alice")
	rec := do(t, h, http.MethodPost, "/api/ambiguous", `{"names":["A.yaml","B.yaml"],"reason":"締め日が祝日のときが未定"}`, jsonHdr)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var got resultsBody
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 2 || !got.Results[0].OK || got.Results[1].OK || got.Results[1].Error == "" {
		t.Fatalf("1 件ずつ結果を返す: %+v", got)
	}
	if len(svc.ambiguous) != 1 || svc.ambiguous[0] != "A.yaml: 締め日が祝日のときが未定" {
		t.Fatalf("理由つきで記録する: %+v", svc.ambiguous)
	}
}

func TestAmbiguous_Guards(t *testing.T) {
	cases := []struct {
		name, body string
		hdr        map[string]string
		want       int
	}{
		{"理由が無い", `{"names":["A.yaml"]}`, jsonHdr, http.StatusBadRequest},
		{"理由が空白だけ", `{"names":["A.yaml"],"reason":"  "}`, jsonHdr, http.StatusBadRequest},
		{"空の names", `{"names":[],"reason":"x"}`, jsonHdr, http.StatusBadRequest},
		{"JSON 以外", `names=A.yaml&reason=x`, map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, http.StatusUnsupportedMediaType},
		{"別 Origin", `{"names":["A.yaml"],"reason":"x"}`, map[string]string{"Content-Type": "application/json", "Origin": "http://evil.example"}, http.StatusForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc := &fakeService{}
			rec := do(t, New(svc, "alice"), http.MethodPost, "/api/ambiguous", c.body, c.hdr)
			if rec.Code != c.want {
				t.Fatalf("status %d, want %d: %s", rec.Code, c.want, rec.Body.String())
			}
			if len(svc.ambiguous) != 0 {
				t.Fatalf("拒否したら記録しない: %+v", svc.ambiguous)
			}
		})
	}
	if rec := do(t, New(&fakeService{}, "alice"), http.MethodGet, "/api/ambiguous", "", nil); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET では記録しない: %d", rec.Code)
	}
}

func TestAssets_AmbiguousButtonUsesTextContent(t *testing.T) {
	b, err := fs.ReadFile(assets, "assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	for _, want := range []string{"曖昧", "/api/ambiguous", "reason"} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js に %q が無い（候補ごとの曖昧ボタンと理由の入力）", want)
		}
	}
	// コメント中の「innerHTML に流さない」は対象外にするため、プロパティ・呼び出しの形で探す
	for _, ng := range []string{".innerHTML", ".outerHTML", "insertAdjacentHTML(", "document.write("} {
		if strings.Contains(js, ng) {
			t.Errorf("サーバの文字列は textContent で入れる。%s は使わない", ng)
		}
	}
}
