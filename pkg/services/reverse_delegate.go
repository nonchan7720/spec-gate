package services

import (
	"context"
	"fmt"
	"slices"

	"github.com/nonchan7720/spec-gate/pkg/domain"
)

// ReverseDelegation は reverse --emit / --collect の結果。
type ReverseDelegation struct {
	// Phase は次に待つ回答の段階（ReversePhaseRestate / ReversePhaseCompare）か、ReversePhaseDone
	Phase string
	// Prompts はサブエージェントに 1 つずつ渡すプロンプトの絶対パス。Done なら空
	Prompts []string
	// Report は Done のときのレポート
	Report *ReverseReport
}

// reverseEntry は claim 1 件の途中経過。
type reverseEntry struct {
	Result domain.ReverseResult `json:"result"`
	// Pending は LLM の回答待ち（false なら Result が確定済み）
	Pending bool `json:"pending"`
	// Excerpt は比較で使う元の仕様の抜粋（emit 時点のもの）
	Excerpt string `json:"excerpt,omitempty"`
}

type reverseManifest struct {
	IDs     []string       `json:"ids"`
	Phase   string         `json:"phase"`
	Entries []reverseEntry `json:"entries"`
}

// Emit は claim ごとに再生成のプロンプトを書き出す。LLM は呼ばない。
// 再生成のプロンプトには元の仕様を含めない（claim だけから書き起こさせる）。
// テキストを持たない・読めない claim は、この時点で skipped / error に確定する。
func (s *ReverseService) Emit(ctx context.Context, ids []string) (ReverseDelegation, error) {
	targets, err := s.targets(ids)
	if err != nil {
		return ReverseDelegation{}, err
	}
	names := map[string]string{}
	for _, c := range targets {
		n := llmName(c.ID)
		if prev, dup := names[n]; dup && prev != c.ID {
			return ReverseDelegation{}, fmt.Errorf("claim %s と %s は作業ディレクトリ内の名前（%s）が衝突する", prev, c.ID, n)
		}
		names[n] = c.ID
	}
	// 前回の emit の回答が残っていると、今回の回答と取り違える
	if err := s.Work.RemoveAll(reverseWorkDir); err != nil {
		return ReverseDelegation{}, err
	}

	m := reverseManifest{IDs: ids, Phase: ReversePhaseRestate}
	var prompts []string
	for _, c := range targets {
		res, excerpt, ok := s.prepare(ctx, c)
		e := reverseEntry{Result: res}
		if ok {
			body, err := s.Prompter.RestatePrompt(c)
			if err != nil {
				e.Result.Status = domain.ReverseError
				e.Result.Detail = fmt.Sprintf("再生成のプロンプトを作れない: %v", err)
			} else {
				p := reversePromptPath(c.ID, ReversePhaseRestate)
				answer := s.Work.Abs(reverseAnswerPath(c.ID, ReversePhaseRestate))
				if err := s.Work.Write(p, taskPrompt(answer, body)); err != nil {
					return ReverseDelegation{}, err
				}
				prompts = append(prompts, s.Work.Abs(p))
				e.Pending = true
				e.Excerpt = excerpt
			}
		}
		m.Entries = append(m.Entries, e)
	}
	if err := writeManifest(s.Work, reverseWorkDir, m); err != nil {
		return ReverseDelegation{}, err
	}
	return ReverseDelegation{Phase: ReversePhaseRestate, Prompts: prompts}, nil
}

// Collect は回答待ちの段階の回答を読む。
// 再生成の回答がそろえば比較のプロンプトを書き出し、比較の回答がそろえばレポートを返して作業ディレクトリを消す。
// 欠けていれば *MissingAnswersError を返し、何も進めない。
func (s *ReverseService) Collect(_ context.Context, ids []string) (ReverseDelegation, error) {
	var m reverseManifest
	found, err := readManifest(s.Work, reverseWorkDir, &m)
	if err != nil {
		return ReverseDelegation{}, err
	}
	if !found {
		return ReverseDelegation{}, fmt.Errorf("逆変換のプロンプトが無い。先に spec-gate reverse --emit を実行する")
	}
	if len(ids) > 0 && !slices.Equal(ids, m.IDs) {
		return ReverseDelegation{}, fmt.Errorf("emit したときの claim（%v）と違う claim（%v）が指定された", m.IDs, ids)
	}

	answers, err := s.pendingAnswers(m)
	if err != nil {
		return ReverseDelegation{}, err
	}

	switch m.Phase {
	case ReversePhaseRestate:
		var prompts []string
		for i := range m.Entries {
			e := &m.Entries[i]
			if !e.Pending {
				continue
			}
			restated, err := s.Prompter.ParseRestate(answers[i])
			if err != nil {
				e.Pending = false
				e.Result.Status = domain.ReverseError
				e.Result.Detail = fmt.Sprintf("仕様文の再生成に失敗: %v", err)
				continue
			}
			e.Result.Restated = restated
			p := reversePromptPath(e.Result.ID, ReversePhaseCompare)
			answer := s.Work.Abs(reverseAnswerPath(e.Result.ID, ReversePhaseCompare))
			if err := s.Work.Write(p, taskPrompt(answer, s.Prompter.ComparePrompt(restated, e.Excerpt))); err != nil {
				return ReverseDelegation{}, err
			}
			prompts = append(prompts, s.Work.Abs(p))
		}
		if len(prompts) > 0 {
			m.Phase = ReversePhaseCompare
			if err := writeManifest(s.Work, reverseWorkDir, m); err != nil {
				return ReverseDelegation{}, err
			}
			return ReverseDelegation{Phase: ReversePhaseCompare, Prompts: prompts}, nil
		}
	case ReversePhaseCompare:
		for i := range m.Entries {
			e := &m.Entries[i]
			if !e.Pending {
				continue
			}
			cmp, err := s.Prompter.ParseCompare(answers[i])
			settleCompare(&e.Result, cmp, err)
			e.Pending = false
		}
	default:
		return ReverseDelegation{}, fmt.Errorf("%s/%s の段階 %q が分からない", reverseWorkDir, manifestName, m.Phase)
	}

	var rep ReverseReport
	for _, e := range m.Entries {
		rep.Results = append(rep.Results, e.Result)
	}
	if err := s.Work.RemoveAll(reverseWorkDir); err != nil {
		return ReverseDelegation{}, err
	}
	return ReverseDelegation{Phase: ReversePhaseDone, Report: &rep}, nil
}

// pendingAnswers は回答待ちの claim の、いまの段階の回答を entries と同じ添字で返す。
// 回答は reverse の作業ディレクトリの、その claim・その段階のファイルからだけ読む。
func (s *ReverseService) pendingAnswers(m reverseManifest) ([]string, error) {
	answers := make([]string, len(m.Entries))
	miss := &MissingAnswersError{}
	for i, e := range m.Entries {
		if !e.Pending {
			continue
		}
		p := reverseAnswerPath(e.Result.ID, m.Phase)
		a, ok, err := s.Work.Read(p)
		if err != nil {
			return nil, err
		}
		if !ok {
			miss.Missing = append(miss.Missing, e.Result.ID+"."+m.Phase)
			miss.Paths = append(miss.Paths, s.Work.Abs(p))
			continue
		}
		answers[i] = a
	}
	if len(miss.Missing) > 0 {
		return nil, miss
	}
	return answers, nil
}
