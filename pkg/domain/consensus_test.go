package domain

import "testing"

func TestNormalizeClaimTitle(t *testing.T) {
	cases := []struct{ in, want string }{
		{"[M-1] 管理者だけが削除できる【要確認: 権限表が無い】", "管理者だけが削除できる"},
		{"`review ask` の N は 1〜4（既定 4）", "reviewaskのnは14既定4"},
		{"比較の変異は `>` → `>=` と `<`", "比較の変異は>>=と<"},
	}
	for _, c := range cases {
		if got := NormalizeClaimTitle(c.in); got != c.want {
			t.Errorf("NormalizeClaimTitle(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTitleSimilarity(t *testing.T) {
	sim := func(a, b string) float64 { return TitleSimilarity(NormalizeClaimTitle(a), NormalizeClaimTitle(b)) }
	if s := sim("提案だけを行い、claim は書き換えない（claim の修正は人間が行う）", "見出しの提案だけを行い、claim は書き換えない（claim の修正は人間が行う）"); s < ParaphraseThreshold {
		t.Errorf("言い換えは閾値以上: %.3f", s)
	}
	if s := sim("check --role engineer は fail だけを出す", "check --role pm は stale と unapproved を出す"); s >= ParaphraseThreshold {
		t.Errorf("別の主張は閾値未満: %.3f", s)
	}
	if s := sim("算術の変異は + → -、- → +、* → /、/ → * の 4 種で全部とする", "比較の変異は > → >= と <、< → <= と >、>= → >、<= → <、== → !=、!= → == で全部とする"); s >= ParaphraseThreshold {
		t.Errorf("演算子だけが違う別の主張は閾値未満: %.3f", s)
	}
	if s := TitleSimilarity("a", "b"); s != 0 {
		t.Errorf("bigram の無い異なる文字列は 0: %.3f", s)
	}
	if s := TitleSimilarity("a", "a"); s != 1 {
		t.Errorf("同じ文字列は 1: %.3f", s)
	}
}

func TestMergeRuns_OneRunAtMostOncePerAgreement(t *testing.T) {
	m := func(title string) Claim {
		return Claim{Kind: KindManual, Title: title, Source: Source{Ref: "a.md#x"}}
	}
	got := MergeRuns([][]Claim{
		{m("変更行数は追加行と削除行の合計とする"), m("trace の変更行数は追加行と削除行の合計とする")},
		{m("変更行数は追加行と削除行の合計とし、git でなければ 0")},
	})
	if len(got) != 2 || got[0].Hits != 2 || got[1].Hits != 1 || len(got[0].Members) != 2 {
		t.Fatalf("1 run は 1 つの束に 1 件まで: %+v", got)
	}
}
