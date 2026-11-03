package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/internal/textutil"
)

// DefaultMutationMax は 1 回の mutation で走らせる変異の既定の上限。
const DefaultMutationMax = 50

// MutationService はプランの verify が甘くないかを、コードに変異を入れて確かめる（mutation score）。
// 変異を入れたファイルは、検出・未検出・エラー・panic・中断（ctx の取り消し）のどれでも元に戻す。
type MutationService struct {
	Plans  domain.PlanRepository
	Claims domain.ClaimRepository
	Files  domain.SourceFiles
	Runner domain.CommandRunner
	// Paths は binding の語がリポジトリに実在するかを答える。変更検知（CheckService.Paths）と同じ基準で
	// 触るパスを選ぶ。nil なら語をすべて渡し、実在しないものは Files.Expand が除く。
	Paths domain.PathChecker
}

// Run はプランファイル path を読んで mutation score を計算する。max は走らせる変異の上限（0 以下なら無制限）。
func (s *MutationService) Run(ctx context.Context, path string, max int) (domain.MutationResult, error) {
	p, err := s.Plans.Load(path)
	if err != nil {
		return domain.MutationResult{}, err
	}
	return s.Score(ctx, p, max)
}

// Score はプラン p の claims が触るファイルに変異を 1 か所ずつ入れ、verify 行で検出できた割合を出す。
func (s *MutationService) Score(ctx context.Context, p domain.Plan, max int) (domain.MutationResult, error) {
	var res domain.MutationResult
	if len(p.Items) == 0 {
		return res, fmt.Errorf("%s: チェック項目がない", p.Path)
	}
	if missing := p.MissingVerify(); len(missing) > 0 {
		return res, fmt.Errorf("%s: verify のない項目がある（L%d）", p.Path, missing[0].Line)
	}
	touches, err := s.touches(p)
	if err != nil {
		return res, err
	}
	files, err := s.Files.Expand(touches)
	if err != nil {
		return res, err
	}

	// 変異を入れる前に verify が通ることを確かめる。落ちるなら「検出」と区別できない
	if failed, out, err := s.verify(ctx, p); err != nil {
		return res, err
	} else if failed != "" {
		return res, fmt.Errorf("変異を入れる前に verify が落ちる。先に verify を通す\n$ %s\n%s", failed, textutil.Tail(out, 15))
	}

	originals := map[string]string{}
	var all []domain.FileMutant
	for _, f := range files {
		src, err := s.Files.Read(f)
		if err != nil {
			return res, err
		}
		originals[f] = src
		for _, m := range domain.MutateSource(src) {
			all = append(all, domain.FileMutant{File: f, Mutant: m})
		}
	}
	res.Candidates = len(all)

	for _, m := range domain.PickEvenly(all, max) {
		killed, err := s.runOne(ctx, p, m, originals[m.File])
		if err != nil {
			return res, err
		}
		res.Total++
		if killed {
			res.Killed++
		} else {
			res.Survivors = append(res.Survivors, m)
		}
	}
	return res, nil
}

// runOne は変異 m を入れて verify を走らせ、元に戻す。復元は defer なので panic でも走る。
func (s *MutationService) runOne(ctx context.Context, p domain.Plan, m domain.FileMutant, orig string) (killed bool, err error) {
	defer func() {
		if rerr := s.Files.Write(m.File, orig); rerr != nil {
			err = errors.Join(err, fmt.Errorf("%s を元に戻せなかった。手で戻すこと: %w", m.File, rerr))
		}
	}()
	if err := s.Files.Write(m.File, m.Apply(orig)); err != nil {
		return false, err
	}
	failed, _, err := s.verify(ctx, p)
	if err != nil {
		return false, err
	}
	return failed != "", nil
}

// verify はプランの verify 行を順に実行し、最初に落ちた行とその出力を返す（全部通れば空）。
// ctx が取り消された（シグナルでの中断など）ときは、落ちた扱いにせずエラーを返す。
func (s *MutationService) verify(ctx context.Context, p domain.Plan) (failed, out string, err error) {
	for _, it := range p.Items {
		if err := ctx.Err(); err != nil {
			return "", "", err
		}
		o, rerr := s.Runner.Run(ctx, it.Verify)
		if err := ctx.Err(); err != nil {
			return "", "", err
		}
		if rerr != nil {
			return it.Verify, o, nil
		}
	}
	return "", "", nil
}

// touches はプランの claims が触るパス（binding のパス・layers・story）。
func (s *MutationService) touches(p domain.Plan) ([]string, error) {
	claims, err := s.Claims.All()
	if err != nil {
		return nil, err
	}
	byID := map[string]domain.Claim{}
	for _, c := range claims {
		byID[c.ID] = c
	}
	var exists func(string) bool
	if s.Paths != nil {
		exists = s.Paths.Exists
	}
	var out []string
	var unknown []string
	for _, id := range p.Claims {
		c, ok := byID[id]
		if !ok {
			unknown = append(unknown, id)
			continue
		}
		out = append(out, c.Touches(exists)...)
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("%s: claim が見つからない: %s", p.Path, strings.Join(unknown, ", "))
	}
	return out, nil
}

// FormatSurvivors は生き残った変異を「file:line before → after」で並べる。
func FormatSurvivors(ms []domain.FileMutant) string {
	var b strings.Builder
	for _, m := range ms {
		fmt.Fprintf(&b, "  %s:%d  %s  →  %s\n", m.File, m.Line, strings.TrimSpace(m.Before), strings.TrimSpace(m.After))
	}
	return b.String()
}
