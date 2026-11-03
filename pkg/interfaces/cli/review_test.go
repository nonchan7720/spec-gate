package cli

import (
	"strings"
	"testing"
)

func TestListenReview_LoopbackOnly(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:0", ":0", "192.0.2.1:0", "example.com:0"} {
		if ln, err := listenReview(addr); err == nil {
			_ = ln.Close()
			t.Errorf("%s はループバックでないので拒否", addr)
		} else if !strings.Contains(err.Error(), "127.0.0.1") {
			t.Errorf("%s: 理由に 127.0.0.1 を挙げる: %v", addr, err)
		}
	}
	ln, err := listenReview("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	if !strings.HasPrefix(reviewURL(ln), "http://127.0.0.1:") {
		t.Fatalf("URL: %s", reviewURL(ln))
	}
}
