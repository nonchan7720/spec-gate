package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/services"
)

// 組み立て済みの app で、holdout が check / lint に流れ、出力に値が出ないことを確かめる。
func TestApp_HoldoutEndToEnd(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "docs/s.md", "# 仕様\n\n## 恒等\n\n入力をそのまま返す。\n")
	writeFile(t, root, "claims/spec/ID-001.yaml", `id: ID-001
kind: example
title: 恒等
source: { type: markdown, ref: "docs/s.md#恒等" }
binding: sed 's/"in"/"out"/'
examples:
  - { in: 1, out: 1 }
properties:
  - "output < 5000"
`)
	// 期待値は out_sha256（31337 / 2 / 4242 の正規化 JSON の SHA-256）だけ
	writeFile(t, root, "holdout/ID-001.yaml", `examples:
  - { in: 31337, out_sha256: 1483099c89000a68c3d88446a6a7669b765f09900cbfb0898ccd784b2a6bfe2d }
  - { in: 2, out_sha256: d4735e3a265e16eee03f59718b9b5d03019c07d8b6c51f90da3a666eec13ab35 }
  - { in: 3, out_sha256: 0315b4020af3eccab7706679580ac87a710d82970733b8719e70af9b57e7b9e6 }
`)
	a := newApp(root)

	// lint: 形だけを見る（期待値の平文が無いので properties との照合はしない）
	issues, _, err := a.lint.Run()
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("lint は holdout の形だけを見る: %+v", issues)
	}

	if _, err := a.approve.Approve(context.Background(), "ID-001", "test"); err != nil {
		t.Fatal(err)
	}
	rep, err := a.check.Run(context.Background(), services.CheckRequest{NoPlans: true})
	if err != nil {
		t.Fatal(err)
	}
	// holdout[0] はハッシュは一致するが実際の出力が property を満たさない、holdout[2] はハッシュが違う
	if len(rep.Claims) != 1 || rep.Claims[0].Status != "fail" {
		t.Fatalf("holdout が不一致なら fail: %+v", rep.Claims)
	}
	var human strings.Builder
	writeReport(&human, rep, false)
	code, hook := hookResult(rep)
	if code != 2 {
		t.Fatalf("hook は止める: %d", code)
	}
	for _, out := range []string{human.String(), hook} {
		if !strings.Contains(out, "holdout 3 件中 2 件が不一致（値は表示しない）") {
			t.Fatalf("件数だけを出す:\n%s", out)
		}
		assertHidden(t, out)
	}
}

func assertHidden(t *testing.T, s string) {
	t.Helper()
	for _, v := range []string{"31337", "4242"} {
		if strings.Contains(s, v) {
			t.Fatalf("holdout の値 %s が出ている:\n%s", v, s)
		}
	}
}

func TestRunHoldoutAdd_WritesHashOnly(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "claims/spec/ID-001.yaml", `id: ID-001
kind: example
title: 恒等
source: { type: markdown, ref: "docs/s.md#恒等" }
binding: sed 's/"in"/"out"/'
examples:
  - { in: 1, out: 1 }
`)
	a := newApp(root)
	var out, errOut strings.Builder
	// 1 件目は標準入力（パイプ）から、2 件目は --out - で標準入力から読む
	for _, c := range []struct{ in, flag string }{{`{"amount": 31337.5}`, ""}, {`2`, "-"}} {
		stdin := strings.NewReader(map[string]string{"": `{"total": 4242, "note": "<円>"}` + "\n", "-": `2`}[c.flag])
		if err := runHoldoutAdd(&out, &errOut, stdin, a, "ID-001", c.in, c.flag, c.flag != ""); err != nil {
			t.Fatal(err)
		}
	}
	if errOut.Len() != 0 {
		t.Fatalf("標準入力から読むときは警告しない: %s", errOut.String())
	}
	b, err := os.ReadFile(filepath.Join(root, "holdout", "ID-001.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "4242") || strings.Contains(string(b), "<円>") || strings.Contains(out.String(), "4242") || strings.Contains(errOut.String(), "4242") {
		t.Fatalf("out の平文は保存も表示もしない:\n%s\n%s", b, out.String())
	}
	exs, err := a.holdout.Holdout("ID-001")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := domain.OutputHash(map[string]any{"note": "<円>", "total": 4242})
	if len(exs) != 2 || exs[0].OutSHA256 != want || !domain.Equal(exs[0].In, map[string]any{"amount": 31337.5}) {
		t.Fatalf("in と out_sha256 を末尾に足す: %+v", exs)
	}
	if !strings.Contains(out.String(), filepath.Join("holdout", "ID-001.yaml")) {
		t.Fatalf("書いたファイルを出す: %s", out.String())
	}
}

func TestRunHoldoutAdd_RejectsBadInput(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "claims/spec/ID-001.yaml", `id: ID-001
kind: example
title: 恒等
source: { type: markdown, ref: "docs/s.md#恒等" }
binding: cat
examples:
  - { in: 1, out: 1 }
`)
	a := newApp(root)
	for _, c := range []struct{ id, in, stdin string }{
		{"ID-001", `{`, `1`},
		{"ID-001", `1`, `not-json-4242`},
		{"ID-001", `1`, ""},
		{"NONE-001", `1`, `1`},
	} {
		var errOut strings.Builder
		err := runHoldoutAdd(&strings.Builder{}, &errOut, strings.NewReader(c.stdin), a, c.id, c.in, "", false)
		if err == nil {
			t.Fatalf("%+v はエラー", c)
		}
		if strings.Contains(err.Error(), "4242") || strings.Contains(errOut.String(), "4242") {
			t.Fatalf("エラーに out の値を出さない: %v", err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "holdout")); !os.IsNotExist(err) {
		t.Fatal("失敗したら何も書かない")
	}
}

func TestRunHoldoutAdd_WarnsOnOutFlag(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "claims/spec/ID-001.yaml", `id: ID-001
kind: example
title: 恒等
source: { type: markdown, ref: "docs/s.md#恒等" }
binding: cat
examples:
  - { in: 1, out: 1 }
`)
	a := newApp(root)
	var out, errOut strings.Builder
	// --out を明示したら標準入力は読まない（後方互換）
	if err := runHoldoutAdd(&out, &errOut, strings.NewReader("not-json"), a, "ID-001", `1`, `4242`, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut.String(), "シェル履歴") || !strings.Contains(errOut.String(), "標準入力") {
		t.Fatalf("--out はシェル履歴に残ると警告する: %q", errOut.String())
	}
	if strings.Contains(out.String()+errOut.String(), "4242") {
		t.Fatalf("out の値を出さない:\n%s\n%s", out.String(), errOut.String())
	}
	exs, err := a.holdout.Holdout("ID-001")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := domain.OutputHash(4242)
	if len(exs) != 1 || exs[0].OutSHA256 != want {
		t.Fatalf("--out の値をハッシュにして足す: %+v", exs)
	}
}
