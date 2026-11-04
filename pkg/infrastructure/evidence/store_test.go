package evidence

import (
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

func TestStore_SaveLoad(t *testing.T) {
	s := New(t.TempDir())
	if _, ok, err := s.Load("A-1"); ok || err != nil {
		t.Fatalf("無ければ ok=false: %v %v", ok, err)
	}
	sha, err := s.Save(domain.Evidence{ID: "A-1", Verdict: domain.AutoOK})
	if err != nil {
		t.Fatal(err)
	}
	raw, ok, err := s.Load("A-1")
	if !ok || err != nil {
		t.Fatalf("書いたものを読める: %v %v", ok, err)
	}
	if got := domain.EvidenceStateOf(raw, sha); !got.SHAMatch || got.Verdict != domain.AutoOK {
		t.Fatalf("書いた sha256 と判定が戻る: %+v", got)
	}
	if got := domain.EvidenceStateOf(append(raw, ' '), sha); got.SHAMatch {
		t.Fatal("中身が変われば sha256 は一致しない")
	}
	for _, bad := range []string{"", "../x", "a/b", ".hidden"} {
		if _, _, err := s.Load(bad); err == nil {
			t.Errorf("不正な ID %q は読まない", bad)
		}
	}
}
