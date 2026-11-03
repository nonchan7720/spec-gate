package domain

import (
	"reflect"
	"sort"
	"testing"
)

func TestIsTouched(t *testing.T) {
	cases := []struct {
		name    string
		touches []string
		changed string
		want    bool
	}{
		// 仕様の表（docs/spec/harness.md「変更検知の精度」）
		{"触るファイルそのもの", []string{"src/payroll/round.ts"}, "src/payroll/round.ts", true},
		{"同じディレクトリ・同じ拡張子の別ファイル", []string{"src/payroll/round.ts"}, "src/payroll/util.ts", true},
		{"同じディレクトリでも拡張子が違えば対象外", []string{"src/payroll/round.ts"}, "src/payroll/README.md", false},
		{"触るディレクトリの配下", []string{"src/app"}, "src/app/handler/x.ts", true},
		{"別ディレクトリは対象外", []string{"src/payroll/round.ts"}, "src/ui/List.tsx", false},

		// 境界
		{"同じ拡張子でもサブディレクトリは別パッケージ", []string{"src/payroll/round.ts"}, "src/payroll/sub/x.ts", false},
		{"同じ拡張子でも親ディレクトリは対象外", []string{"src/payroll/round.ts"}, "src/x.ts", false},
		{"名前が前方一致するだけの兄弟ディレクトリは配下ではない", []string{"src/app"}, "src/application/x.ts", false},
		{"拡張子の無いパス同士は同じディレクトリでも対象外", []string{"tools/run"}, "tools/Makefile", false},
		{"触るディレクトリそのもの", []string{"src/app"}, "src/app", true},
		{"末尾のスラッシュと ./ は無視", []string{"./src/app/"}, "src/app/x.ts", true},
		{"変更側の ./ も無視", []string{"src/payroll/round.ts"}, "./src/payroll/util.ts", true},
		{"ルート直下のファイル同士（同じ拡張子）", []string{"main.go"}, "util.go", true},
		{"URL は触るパスにしない", []string{"https://example.com/a.ts"}, "https://example.com/b.ts", false},
		{"空のパスは無視", []string{""}, "a.ts", false},
		{"どれか 1 つに当たれば対象", []string{"docs/spec/a.md", "src/app"}, "src/app/x.ts", true},
		{"触るパスが無ければ対象外", nil, "src/app/x.ts", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsTouched(tc.touches, tc.changed); got != tc.want {
				t.Fatalf("IsTouched(%q, %q) = %v, want %v", tc.touches, tc.changed, got, tc.want)
			}
		})
	}
}

func TestClaimTouches(t *testing.T) {
	existing := map[string]bool{"tools/call.ts": true, "src/payroll/round.ts": true, "tools/bind": true, "main.go": true}
	exists := func(p string) bool { return existing[p] }

	cases := []struct {
		name  string
		claim Claim
		want  []string
	}{
		{
			"binding の語のうち実在するものだけ",
			Claim{Binding: "npx tsx tools/call.ts src/payroll/round.ts roundYen"},
			[]string{"src/payroll/round.ts", "tools/call.ts"},
		},
		{
			"実在しないパスらしき語は触るパスにしない",
			Claim{Binding: "npx tsx tools/call.ts src/gone.ts f"},
			[]string{"tools/call.ts"},
		},
		{
			"スラッシュの無い語でも実在するファイルなら拾う / ./ は外す",
			Claim{Binding: "go run ./tools/bind relink main.go"},
			[]string{"main.go", "tools/bind"},
		},
		{
			"引用符は外す",
			Claim{Binding: `npx tsx 'tools/call.ts' "src/payroll/round.ts" f`},
			[]string{"src/payroll/round.ts", "tools/call.ts"},
		},
		{
			"URL とフラグは確かめない",
			Claim{Binding: "curl -s https://example.com/tools/call.ts --data=main.go"},
			nil,
		},
		{
			"story と layers は実在に関わらずこれまでどおり",
			Claim{Story: "src/ui/List.stories.tsx", Layers: map[string]string{"app": "src/app"}},
			[]string{"src/app", "src/ui/List.stories.tsx"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.claim.Touches(exists)
			sort.Strings(got)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Touches = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestClaimTouches_NoExistenceCheckKeepsEveryWord(t *testing.T) {
	// 実在を確かめられないときは見逃さない側に倒し、フラグと URL 以外の語をすべて触るパスにする
	got := Claim{Binding: "npx tools/call.ts -v https://example.com/x"}.Touches(nil)
	sort.Strings(got)
	if want := []string{"npx", "tools/call.ts"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Touches(nil) = %q, want %q", got, want)
	}
}
