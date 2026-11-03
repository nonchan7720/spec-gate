package claimrepo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMarkAmbiguous_MovesWithReasonHeader(t *testing.T) {
	root := t.TempDir()
	r := New(root)
	if _, err := r.SavePending(pendingClaim("N-1"), "2/3"); err != nil {
		t.Fatal(err)
	}
	path, err := r.MarkAmbiguous("N-1.yaml", "  締め日が\n祝日のときが決まっていない  ")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "claims", "_ambiguous", "N-1.yaml")
	if path != want {
		t.Fatalf("claims/_ambiguous/ に移る: %q", path)
	}
	if _, err := os.Stat(filepath.Join(root, "claims", "_pending", "N-1.yaml")); !os.IsNotExist(err) {
		t.Fatalf("_pending からは消える: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	first, rest, _ := strings.Cut(string(b), "\n")
	if first != "# ambiguous: 締め日が 祝日のときが決まっていない" {
		t.Fatalf("1 行目に理由を 1 行で記録する（改行はヘッダを壊すので空白にする）: %q", first)
	}
	if !strings.Contains(rest, "# confidence: 2/3") || !strings.Contains(rest, "id: N-1") {
		t.Fatalf("元の内容（confidence ヘッダと claim）は残す: %q", rest)
	}

	got, err := r.Ambiguous()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "N-1.yaml" || got[0].Claim.ID != "N-1" || got[0].Reason != "締め日が 祝日のときが決まっていない" || got[0].Error != "" {
		t.Fatalf("Ambiguous は理由つきで返す: %+v", got)
	}
	if got[0].Path != filepath.Join("claims", "_ambiguous", "N-1.yaml") {
		t.Fatalf("Path はルートからの相対: %q", got[0].Path)
	}

	claims, err := r.All()
	if err != nil || len(claims) != 0 {
		t.Fatalf("_ambiguous は _pending と同じく check の対象外: %+v %v", claims, err)
	}
}

func TestMarkAmbiguous_Rejects(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "claims/_pending/N-1.yaml", manual("N-1"))
	writeFile(t, root, "claims/_pending/N-2.yaml", manual("N-2"))
	writeFile(t, root, "claims/_ambiguous/N-2.yaml", manual("N-2"))
	r := New(root)
	cases := []struct{ name, reason, want string }{
		{"N-1.yaml", "", "理由"},
		{"N-1.yaml", " \n\t", "理由"},
		{"missing.yaml", "x", "ない"},
		{"../spec/N-1.yaml", "x", "不正"},
		{"N-2.yaml", "x", "既に"},
	}
	for _, c := range cases {
		if _, err := r.MarkAmbiguous(c.name, c.reason); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("MarkAmbiguous(%q, %q) は %q を含むエラー: %v", c.name, c.reason, c.want, err)
		}
	}
	for _, n := range []string{"N-1.yaml", "N-2.yaml"} {
		if _, err := os.Stat(filepath.Join(root, "claims", "_pending", n)); err != nil {
			t.Fatalf("失敗時は _pending に残る: %v", err)
		}
	}
}

func TestPromote_RejectsAmbiguousArea(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "claims/_pending/N-1.yaml", manual("N-1"))
	if _, err := New(root).Promote("N-1.yaml", "_ambiguous"); err == nil || !strings.Contains(err.Error(), "不正") {
		t.Fatalf("承認で _ambiguous には移さない: %v", err)
	}
}

func TestAmbiguous_NoDirAndBroken(t *testing.T) {
	got, err := New(t.TempDir()).Ambiguous()
	if err != nil || len(got) != 0 {
		t.Fatalf("_ambiguous が無ければ空: %v %v", got, err)
	}
	root := t.TempDir()
	writeFile(t, root, "claims/_ambiguous/B.yaml", "# ambiguous: 未定\nid: B-1\nunknown_key: 1\n")
	got, err = New(root).Ambiguous()
	if err != nil || len(got) != 1 || got[0].Error == "" || got[0].Reason != "未定" {
		t.Fatalf("読めないものもエラーつきで返す: %+v %v", got, err)
	}
}
