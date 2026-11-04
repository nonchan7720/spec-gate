package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// extractManifest は extract --emit が作業ディレクトリに残す、collect に必要な情報。
type extractManifest struct {
	Ref  string            `json:"ref"`
	Type domain.SourceType `json:"type"`
	Runs int               `json:"runs"`
}

// Emit は抽出のプロンプトを run ごとに 1 つ書き出し、その絶対パスを返す。LLM は呼ばない。
// 各プロンプトは自己完結している（抽出のルール・仕様の抜粋・その run の回答ファイルの絶対パス）。
// サブエージェントは自分のプロンプトだけを読んで答えればよく、他の run を見る必要が無い（合議の独立性）。
func (s *ExtractService) Emit(ctx context.Context, srcType domain.SourceType, ref string, runs int) ([]string, error) {
	if runs < 1 {
		return nil, fmt.Errorf("--runs は 1 以上（%d）", runs)
	}
	ad, ok := s.Adapters[srcType]
	if !ok {
		return nil, fmt.Errorf("source type %s の adapter が未登録", srcType)
	}
	snap, err := ad.Snapshot(ctx, ref)
	if err != nil {
		return nil, err
	}
	if snap.Skipped != "" {
		return nil, fmt.Errorf("仕様を読めない: %s", snap.Skipped)
	}

	dir := extractDir(ref)
	var prev extractManifest
	found, err := readManifest(s.Work, dir, &prev)
	if err != nil {
		return nil, err
	}
	if found && prev.Ref != ref {
		return nil, fmt.Errorf("作業ディレクトリ %s は別の ref（%s）が使っている。先にそちらを collect するか、ディレクトリを消す", dir, prev.Ref)
	}
	// 前回の emit の回答が残っていると、今回の回答と取り違える
	if err := s.Work.RemoveAll(dir); err != nil {
		return nil, err
	}
	if err := writeManifest(s.Work, dir, extractManifest{Ref: ref, Type: srcType, Runs: runs}); err != nil {
		return nil, err
	}
	body := s.Prompter.Prompt(ref, snap.Excerpt)
	paths := ExtractPromptPaths(ref, runs)
	out := make([]string, 0, runs)
	for i, p := range paths {
		if err := s.Work.Write(p, taskPrompt(s.Work.Abs(extractAnswerPath(ref, i+1)), body)); err != nil {
			return nil, err
		}
		out = append(out, s.Work.Abs(p))
	}
	return out, nil
}

// Collect は ref の作業ディレクトリにある run ごとの回答を読み、Extract と同じ合議で claims/_pending/ に置く。
// 回答は ref のディレクトリからだけ読む（別の ref の回答を混ぜない）。
// 欠けていれば *MissingAnswersError を返し、何も置かない。成功したら ref の作業ディレクトリを消す。
func (s *ExtractService) Collect(_ context.Context, ref string) ([]Candidate, error) {
	dir := extractDir(ref)
	var m extractManifest
	found, err := readManifest(s.Work, dir, &m)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%s の抽出プロンプトが無い。先に spec-gate extract %q --emit を実行する", ref, ref)
	}
	if m.Ref != ref {
		return nil, fmt.Errorf("作業ディレクトリ %s は別の ref（%s）のもの", dir, m.Ref)
	}

	answers := make([]string, m.Runs)
	miss := &MissingAnswersError{}
	for k := 1; k <= m.Runs; k++ {
		p := extractAnswerPath(ref, k)
		a, ok, err := s.Work.Read(p)
		if err != nil {
			return nil, err
		}
		if !ok {
			miss.Missing = append(miss.Missing, fmt.Sprintf("run-%d", k))
			miss.Paths = append(miss.Paths, s.Work.Abs(p))
			continue
		}
		answers[k-1] = a
	}
	if len(miss.Missing) > 0 {
		return nil, miss
	}

	perRun := make([][]domain.Claim, 0, m.Runs)
	var errs []error
	for k, a := range answers {
		claims, err := s.Prompter.Parse(a, ref)
		if err != nil {
			errs = append(errs, fmt.Errorf("run-%d の回答（%s）: %w", k+1, s.Work.Abs(extractAnswerPath(ref, k+1)), err))
			continue
		}
		perRun = append(perRun, claims)
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	cands, err := s.consensus(m.Type, perRun)
	if err != nil {
		return nil, err
	}
	if err := s.Work.RemoveAll(dir); err != nil {
		return nil, err
	}
	return cands, nil
}
