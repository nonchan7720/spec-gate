package services

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// fakeFiles はメモリ上のソースファイル。
type fakeFiles struct {
	files    map[string]string
	expanded [][]string
	writes   int
}

func (f *fakeFiles) Expand(paths []string) ([]string, error) {
	f.expanded = append(f.expanded, paths)
	var out []string
	for _, p := range paths {
		if _, ok := f.files[p]; ok && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (f *fakeFiles) Read(path string) (string, error) {
	s, ok := f.files[path]
	if !ok {
		return "", errors.New("not found")
	}
	return s, nil
}

func (f *fakeFiles) Write(path, content string) error {
	f.writes++
	f.files[path] = content
	return nil
}

// funcRunner は verify の実行を関数で差し替える。
type funcRunner func(ctx context.Context, command string) (string, error)

func (f funcRunner) Run(ctx context.Context, command string) (string, error) { return f(ctx, command) }

const calcSrc = "package calc\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n\nfunc Positive(x int) bool {\n\treturn x > 0\n}\n"

// killWhen は、対象ファイルに needles のどれかが含まれると落ちる verify（＝その変異を検出できるテスト）。
func killWhen(files *fakeFiles, needles ...string) funcRunner {
	return func(_ context.Context, _ string) (string, error) {
		for _, n := range needles {
			if strings.Contains(files.files["calc/calc.go"], n) {
				return "FAIL", errors.New("exit status 1")
			}
		}
		return "ok", nil
	}
}

func mutationFixture() (*fakePlans, *fakeClaims, *fakeFiles) {
	plan := planWith("plans/a.md", "go test ./calc")
	plan.Claims = []string{"CALC-001"}
	plans := &fakePlans{plans: map[string]domain.Plan{"plans/a.md": plan}, locks: map[string]*domain.PlanLock{}}
	claims := &fakeClaims{claims: []domain.Claim{
		{ID: "CALC-001", Kind: domain.KindExample, Binding: "go run ./tools/x calc/calc.go"},
		{ID: "OTHER-001", Kind: domain.KindExample, Binding: "go run ./tools/x other/other.go"},
	}}
	files := &fakeFiles{files: map[string]string{"calc/calc.go": calcSrc, "other/other.go": "return a + b\n"}}
	return plans, claims, files
}

func TestMutate_ScoreAndSurvivors(t *testing.T) {
	plans, claims, files := mutationFixture()
	s := &MutationService{Plans: plans, Claims: claims, Files: files, Runner: killWhen(files, "a - b", "x < 0"),
		Paths: fakePaths{"tools/x": true, "calc/calc.go": true, "other/other.go": true}}
	r, err := s.Run(context.Background(), "plans/a.md", 0)
	if err != nil {
		t.Fatal(err)
	}
	if r.Total != 3 || r.Killed != 2 || r.Candidates != 3 {
		t.Fatalf("3 件中 2 件を検出: %+v", r)
	}
	want := []domain.FileMutant{{File: "calc/calc.go", Line: 8, Before: "\treturn x > 0", After: "\treturn x >= 0"}}
	if !slices.Equal(r.Survivors, want) {
		t.Fatalf("生き残った変異: %+v", r.Survivors)
	}
	if files.files["calc/calc.go"] != calcSrc {
		t.Fatalf("終わったら元に戻す: %q", files.files["calc/calc.go"])
	}
	// プランの claims が触るファイルだけを対象にする
	if len(files.expanded) != 1 || !slices.Equal(files.expanded[0], []string{"tools/x", "calc/calc.go"}) {
		t.Fatalf("プランの claim の Touches（変更検知と同じく実在する語だけ）を展開する: %v", files.expanded)
	}
}

func TestMutate_RestoresAfterEveryMutant(t *testing.T) {
	plans, claims, files := mutationFixture()
	var seen []string
	runner := funcRunner(func(_ context.Context, _ string) (string, error) {
		seen = append(seen, files.files["calc/calc.go"])
		if strings.Contains(files.files["calc/calc.go"], "a - b") {
			return "", errors.New("exit status 1")
		}
		return "", nil
	})
	s := &MutationService{Plans: plans, Claims: claims, Files: files, Runner: runner}
	if _, err := s.Run(context.Background(), "plans/a.md", 0); err != nil {
		t.Fatal(err)
	}
	// 変異前の基準実行 1 回 + 変異 3 件。各変異は 1 か所だけ（他の変異が残っていない）
	if len(seen) != 4 || seen[0] != calcSrc {
		t.Fatalf("基準実行と変異ごとの実行: %d 回", len(seen))
	}
	for i, src := range seen[1:] {
		if diff := countDiffLines(calcSrc, src); diff != 1 {
			t.Fatalf("変異 %d は 1 行だけ違うはず（前の変異が戻っていない）: %d 行", i, diff)
		}
	}
	if files.files["calc/calc.go"] != calcSrc {
		t.Fatal("終わったら元に戻す")
	}
}

func countDiffLines(a, b string) int {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	n := 0
	for i := range al {
		if i >= len(bl) || al[i] != bl[i] {
			n++
		}
	}
	return n
}

func TestMutate_RestoresWhenVerifyPanics(t *testing.T) {
	plans, claims, files := mutationFixture()
	runner := funcRunner(func(_ context.Context, _ string) (string, error) {
		if files.files["calc/calc.go"] != calcSrc {
			panic("verify crashed")
		}
		return "", nil
	})
	s := &MutationService{Plans: plans, Claims: claims, Files: files, Runner: runner}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("panic はそのまま上に伝える")
			}
		}()
		_, _ = s.Run(context.Background(), "plans/a.md", 0)
	}()
	if files.files["calc/calc.go"] != calcSrc {
		t.Fatalf("panic しても元に戻す: %q", files.files["calc/calc.go"])
	}
}

func TestMutate_RestoresWhenInterrupted(t *testing.T) {
	plans, claims, files := mutationFixture()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := funcRunner(func(ctx context.Context, _ string) (string, error) {
		if files.files["calc/calc.go"] != calcSrc {
			cancel() // シグナル（Ctrl-C）で中断された状況
			return "", ctx.Err()
		}
		return "", nil
	})
	s := &MutationService{Plans: plans, Claims: claims, Files: files, Runner: runner}
	_, err := s.Run(ctx, "plans/a.md", 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("中断はエラーにする（検出扱いにしない）: %v", err)
	}
	if files.files["calc/calc.go"] != calcSrc {
		t.Fatalf("中断しても元に戻す: %q", files.files["calc/calc.go"])
	}
}

func TestMutate_MaxBoundsTheRun(t *testing.T) {
	plans, claims, files := mutationFixture()
	s := &MutationService{Plans: plans, Claims: claims, Files: files, Runner: killWhen(files)}
	r, err := s.Run(context.Background(), "plans/a.md", 1)
	if err != nil {
		t.Fatal(err)
	}
	if r.Candidates != 3 || r.Total != 1 {
		t.Fatalf("--max 1 なら 1 件だけ走らせる: %+v", r)
	}
}

func TestMutate_BaselineFailureIsAnError(t *testing.T) {
	plans, claims, files := mutationFixture()
	s := &MutationService{Plans: plans, Claims: claims, Files: files, Runner: funcRunner(func(context.Context, string) (string, error) {
		return "FAIL", errors.New("exit status 1")
	})}
	_, err := s.Run(context.Background(), "plans/a.md", 0)
	if err == nil || !strings.Contains(err.Error(), "変異を入れる前") {
		t.Fatalf("変異前に verify が落ちるなら score は意味がないのでエラー: %v", err)
	}
	if files.writes != 0 {
		t.Fatal("基準実行が落ちたらファイルに触らない")
	}
}

func TestMutate_UnknownClaimIsAnError(t *testing.T) {
	plans, claims, files := mutationFixture()
	p := plans.plans["plans/a.md"]
	p.Claims = []string{"NOPE-001"}
	plans.plans["plans/a.md"] = p
	s := &MutationService{Plans: plans, Claims: claims, Files: files, Runner: killWhen(files)}
	if _, err := s.Run(context.Background(), "plans/a.md", 0); err == nil || !strings.Contains(err.Error(), "NOPE-001") {
		t.Fatalf("存在しない claim はエラー: %v", err)
	}
}

func TestMutate_MissingVerifyIsAnError(t *testing.T) {
	plans, claims, files := mutationFixture()
	p := planWith("plans/a.md", "go test ./calc", "")
	p.Claims = []string{"CALC-001"}
	plans.plans["plans/a.md"] = p
	s := &MutationService{Plans: plans, Claims: claims, Files: files, Runner: killWhen(files)}
	if _, err := s.Run(context.Background(), "plans/a.md", 0); err == nil {
		t.Fatal("verify のない項目があれば走らせない")
	}
}

func freezeFixture(th *float64, runner domain.CommandRunner) (*PlanService, *fakePlans, *fakeFiles) {
	plans, claims, files := mutationFixture()
	p := plans.plans["plans/a.md"]
	p.Mutation = th
	plans.plans["plans/a.md"] = p
	if runner == nil {
		runner = killWhen(files, "a - b", "x < 0")
	}
	s := &PlanService{
		Plans:    plans,
		Clock:    fixedClock("2026-11-04T00:00:00Z"),
		Mutation: &MutationService{Plans: plans, Claims: claims, Files: files, Runner: runner},
	}
	return s, plans, files
}

func TestFreeze_RefusesWhenScoreBelowThreshold(t *testing.T) {
	s, plans, files := freezeFixture(new(0.8), nil) // score は 2/3
	_, r, err := s.Freeze(context.Background(), "plans/a.md", "alice")
	if err == nil {
		t.Fatal("score 0.67 < 0.8 なら凍結しない")
	}
	for _, want := range []string{"0.67", "0.8", "calc/calc.go:8", "return x > 0", "return x >= 0"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("理由に %q が無い: %v", want, err)
		}
	}
	if r == nil || r.Killed != 2 {
		t.Fatalf("結果も返す: %+v", r)
	}
	if plans.locks["plans/a.md"] != nil {
		t.Fatal("拒否したらロックを書かない")
	}
	if files.files["calc/calc.go"] != calcSrc {
		t.Fatal("元に戻す")
	}
}

func TestFreeze_FreezesWhenScoreMeetsThreshold(t *testing.T) {
	s, plans, _ := freezeFixture(new(0.6), nil)
	_, r, err := s.Freeze(context.Background(), "plans/a.md", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if r == nil || r.Total != 3 {
		t.Fatalf("score を計算した結果を返す: %+v", r)
	}
	if plans.locks["plans/a.md"] == nil {
		t.Fatal("閾値以上なら凍結する")
	}
}

func TestFreeze_WithoutThresholdDoesNotMutate(t *testing.T) {
	called := false
	s, plans, _ := freezeFixture(nil, funcRunner(func(context.Context, string) (string, error) {
		called = true
		return "", nil
	}))
	_, r, err := s.Freeze(context.Background(), "plans/a.md", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if called || r != nil {
		t.Fatal("mutation が無ければ score を計算しない")
	}
	if plans.locks["plans/a.md"] == nil {
		t.Fatal("凍結する")
	}
}

func TestFreeze_RefusesWhenNoMutants(t *testing.T) {
	s, plans, files := freezeFixture(new(0.5), nil)
	files.files["calc/calc.go"] = "package calc\n"
	_, _, err := s.Freeze(context.Background(), "plans/a.md", "alice")
	if err == nil || !strings.Contains(err.Error(), "変異") {
		t.Fatalf("変異を 1 件も作れなければ score を示せないので凍結しない: %v", err)
	}
	if plans.locks["plans/a.md"] != nil {
		t.Fatal("ロックを書かない")
	}
}

func TestFreeze_ThresholdWithoutMutationServiceIsAnError(t *testing.T) {
	s, plans, _ := freezeFixture(new(0.5), nil)
	s.Mutation = nil
	if _, _, err := s.Freeze(context.Background(), "plans/a.md", "alice"); err == nil {
		t.Fatal("score を計算できないなら凍結しない")
	}
	if plans.locks["plans/a.md"] != nil {
		t.Fatal("ロックを書かない")
	}
}
