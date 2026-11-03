package claimrepo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

func pendingClaim(id string) domain.Claim {
	return domain.Claim{ID: id, Kind: domain.KindManual, Title: "t", Source: domain.Source{Type: "markdown", Ref: "a.md"}}
}

func TestPending_ListsWithConfidence(t *testing.T) {
	root := t.TempDir()
	r := New(root)
	if _, err := r.SavePending(pendingClaim("N-2"), "1/3"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.SavePending(pendingClaim("N-1"), "3/3"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "claims/spec/X.yaml", manual("X-1"))
	writeFile(t, root, "claims/_pending/README.md", "# not a claim\n")

	got, err := r.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "N-1.yaml" || got[1].Name != "N-2.yaml" {
		t.Fatalf("_pending の yaml だけをファイル名順: %+v", got)
	}
	if got[0].Claim.ID != "N-1" || got[0].Confidence != "3/3" || got[1].Confidence != "1/3" {
		t.Fatalf("claim と confidence ヘッダが読める: %+v", got)
	}
	if got[0].Error != "" {
		t.Fatalf("正しい claim にエラーは付かない: %q", got[0].Error)
	}
	if got[0].Path != filepath.Join("claims", "_pending", "N-1.yaml") {
		t.Fatalf("Path はルートからの相対: %q", got[0].Path)
	}
}

func TestPending_BrokenFileIsReportedNotHidden(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "claims/_pending/B.yaml", "# confidence: 2/3\nid: B-1\nkind: manual\n")
	got, err := New(root).Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Error == "" || got[0].Confidence != "2/3" {
		t.Fatalf("壊れた候補もエラーつきで一覧に出す（黙って消さない）: %+v", got)
	}
}

func TestPending_NoDir(t *testing.T) {
	got, err := New(t.TempDir()).Pending()
	if err != nil || len(got) != 0 {
		t.Fatalf("_pending が無ければ空: %v %v", got, err)
	}
}

func TestPromoteAndDemote(t *testing.T) {
	root := t.TempDir()
	r := New(root)
	if _, err := r.SavePending(pendingClaim("N-1"), "2/3"); err != nil {
		t.Fatal(err)
	}
	path, err := r.Promote("N-1.yaml", "spec")
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(root, "claims", "spec", "N-1.yaml") {
		t.Fatalf("claims/<area>/ に移る: %q", path)
	}
	if _, err := os.Stat(filepath.Join(root, "claims", "_pending", "N-1.yaml")); !os.IsNotExist(err) {
		t.Fatalf("_pending からは消える: %v", err)
	}
	claims, err := r.All()
	if err != nil || len(claims) != 1 || claims[0].ID != "N-1" {
		t.Fatalf("移した claim は All に入る: %+v %v", claims, err)
	}

	if err := r.Demote(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Demote で領域から消える: %v", err)
	}
	pending, _ := r.Pending()
	if len(pending) != 1 || pending[0].Confidence != "2/3" {
		t.Fatalf("Demote で _pending にヘッダごと戻る: %+v", pending)
	}
}

func TestPromote_Rejects(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "claims/_pending/N-1.yaml", manual("N-1"))
	writeFile(t, root, "claims/spec/N-1.yaml", manual("OTHER"))
	r := New(root)
	cases := []struct{ name, area, want string }{
		{"N-1.yaml", "spec", "既に"},
		{"missing.yaml", "spec", "ない"},
		{"../spec/N-1.yaml", "ui", "不正"},
		{"N-1.yaml", "../x", "不正"},
		{"N-1.yaml", "_pending", "不正"},
		{"N-1.yaml", "", "不正"},
	}
	for _, c := range cases {
		if _, err := r.Promote(c.name, c.area); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("Promote(%q, %q) は %q を含むエラー: %v", c.name, c.area, c.want, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "claims", "_pending", "N-1.yaml")); err != nil {
		t.Fatalf("失敗時は _pending に残る: %v", err)
	}
}

func TestDemote_OnlyInsideClaims(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "other/N-1.yaml", manual("N-1"))
	if err := New(root).Demote(filepath.Join(root, "other", "N-1.yaml")); err == nil {
		t.Fatal("claims/ の外のファイルは戻さない")
	}
}

func TestDiscardPending(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "claims/_pending/N-1.yaml", manual("N-1"))
	r := New(root)
	if err := r.DiscardPending("../_pending/N-1.yaml"); err == nil {
		t.Fatal("パスを含む名前は拒否")
	}
	if err := r.DiscardPending("N-1.yaml"); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.Pending(); len(got) != 0 {
		t.Fatalf("却下で消える: %+v", got)
	}
	if err := r.DiscardPending("N-1.yaml"); err == nil {
		t.Fatal("無い候補はエラー")
	}
}
