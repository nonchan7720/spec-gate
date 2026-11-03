package claimrepo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

func writeFile(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func manual(id string) string {
	return "id: " + id + "\nkind: manual\ntitle: t\nsource:\n  type: markdown\n  ref: docs/spec.md#a\n"
}

func TestAll_SortedAndSkipsPending(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "claims/spec/B.yaml", manual("B-1"))
	writeFile(t, root, "claims/arch/A.yml", manual("A-1"))
	writeFile(t, root, "claims/_pending/P.yaml", manual("P-1"))
	writeFile(t, root, "claims/SCHEMA.md", "# not a claim\n")
	claims, err := New(root).All()
	if err != nil {
		t.Fatal(err)
	}
	if len(claims) != 2 || claims[0].ID != "A-1" || claims[1].ID != "B-1" {
		t.Fatalf("_pending と yaml 以外を除き ID 順: %+v", claims)
	}
	if claims[0].File != filepath.Join(root, "claims/arch/A.yml") {
		t.Fatalf("File に読み込み元が入る: %q", claims[0].File)
	}
}

func TestAll_NoClaimsDir(t *testing.T) {
	claims, err := New(t.TempDir()).All()
	if err != nil || claims != nil {
		t.Fatalf("claims/ が無ければ空: %v %v", claims, err)
	}
}

func TestAll_ErrorsNameTheFile(t *testing.T) {
	cases := map[string]string{
		"壊れた YAML":    "id: X\nkind: [\n",
		"必須項目が無い":     "id: X\nkind: manual\n",
		"未知のキー（綴り違い）": manual("X") + "forbiden:\n  - a -> b\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, "claims/spec/X.yaml", body)
			_, err := New(root).All()
			if err == nil || !strings.Contains(err.Error(), filepath.Join("claims", "spec", "X.yaml")) {
				t.Fatalf("読めない claim はファイル名つきのエラー（黙って無視しない）: %v", err)
			}
		})
	}
}

func TestAll_DuplicateID(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "claims/a/X.yaml", manual("X-1"))
	writeFile(t, root, "claims/b/X.yaml", manual("X-1"))
	if _, err := New(root).All(); err == nil || !strings.Contains(err.Error(), "重複") {
		t.Fatalf("ID の重複はエラー: %v", err)
	}
}

func TestSave_RoundTripWithApproval(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "claims/spec/X.yaml", manual("X-1"))
	r := New(root)
	claims, err := r.All()
	if err != nil {
		t.Fatal(err)
	}
	c := claims[0]
	c.Approved = &domain.Approval{By: "alice", At: "2026-11-04T00:00:00Z", SourceHash: "sha256:abc"}
	if err := r.Save(c); err != nil {
		t.Fatal(err)
	}
	again, err := r.All()
	if err != nil {
		t.Fatal(err)
	}
	if again[0].Approved == nil || *again[0].Approved != *c.Approved {
		t.Fatalf("approved が保存される: %+v", again[0].Approved)
	}
	if err := r.Save(domain.Claim{ID: "Y"}); err == nil {
		t.Fatal("保存先が不明な claim はエラー")
	}
}

func TestSavePending_HeaderAndExcludedFromAll(t *testing.T) {
	root := t.TempDir()
	r := New(root)
	path, err := r.SavePending(domain.Claim{ID: "N-1", Kind: domain.KindManual, Title: "t", Source: domain.Source{Type: "markdown", Ref: "a.md"}}, "2/3")
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join("claims", "_pending", "N-1.yaml") {
		t.Fatalf("path = %q", path)
	}
	b, _ := os.ReadFile(filepath.Join(root, path))
	if !strings.HasPrefix(string(b), "# confidence: 2/3\n") {
		t.Fatalf("合議の一致数がヘッダに入る: %q", string(b))
	}
	if claims, _ := r.All(); len(claims) != 0 {
		t.Fatalf("_pending は check 対象外: %+v", claims)
	}
}
