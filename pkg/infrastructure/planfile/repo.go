// Package planfile は plans/*.md の読み込みと .spec-gate/plan.lock.json の管理。
package planfile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"gopkg.in/yaml.v3"
)

type Repo struct{ root string }

func New(root string) *Repo { return &Repo{root: root} }

func (r *Repo) lockPath() string { return filepath.Join(r.root, ".spec-gate", "plan.lock.json") }

func (r *Repo) All() ([]domain.Plan, error) {
	files, _ := filepath.Glob(filepath.Join(r.root, "plans", "*.md"))
	sort.Strings(files)
	var plans []domain.Plan
	for _, f := range files {
		if strings.EqualFold(filepath.Base(f), "TEMPLATE.md") {
			continue
		}
		rel, _ := filepath.Rel(r.root, f)
		p, err := r.Load(rel)
		if err != nil {
			return nil, err
		}
		plans = append(plans, p)
	}
	return plans, nil
}

var (
	itemRe   = regexp.MustCompile(`^\s*[-*]\s+\[([ xX])\]\s+(.*)$`)
	verifyRe = regexp.MustCompile(`^\s+verify:\s*(.+)$`)
)

func (r *Repo) Load(path string) (domain.Plan, error) {
	b, err := os.ReadFile(filepath.Join(r.root, path))
	if err != nil {
		return domain.Plan{}, err
	}
	p := domain.Plan{Path: path, Status: "draft"}
	body := string(b)
	// lineOffset は front matter として取り除いた行数。Line をファイル上の行番号にするために足す。
	lineOffset := 0

	// front matter
	if strings.HasPrefix(body, "---\n") {
		if end := strings.Index(body[4:], "\n---"); end >= 0 {
			var fm struct {
				Title  string   `yaml:"title"`
				Status string   `yaml:"status"`
				Claims []string `yaml:"claims"`
			}
			_ = yaml.Unmarshal([]byte(body[4:4+end]), &fm)
			p.Title, p.Claims = fm.Title, fm.Claims
			if fm.Status != "" {
				p.Status = fm.Status
			}
			lineOffset = strings.Count(body[:4+end+4], "\n")
			body = body[4+end+4:]
		}
	}

	var cur *domain.PlanItem
	for i, line := range strings.Split(body, "\n") {
		if m := itemRe.FindStringSubmatch(line); m != nil {
			if cur != nil {
				p.Items = append(p.Items, *cur)
			}
			cur = &domain.PlanItem{Text: strings.TrimSpace(m[2]), Line: lineOffset + i + 1, Done: m[1] != " "}
			continue
		}
		if cur != nil {
			if m := verifyRe.FindStringSubmatch(line); m != nil {
				cur.Verify = strings.TrimSpace(m[1])
			}
		}
	}
	if cur != nil {
		p.Items = append(p.Items, *cur)
	}
	return p, nil
}

func (r *Repo) readLocks() (map[string]domain.PlanLock, error) {
	locks := map[string]domain.PlanLock{}
	b, err := os.ReadFile(r.lockPath())
	if os.IsNotExist(err) {
		return locks, nil
	}
	if err != nil {
		return nil, err
	}
	return locks, json.Unmarshal(b, &locks)
}

func (r *Repo) Lock(path string) (*domain.PlanLock, error) {
	locks, err := r.readLocks()
	if err != nil {
		return nil, err
	}
	if l, ok := locks[path]; ok {
		return &l, nil
	}
	return nil, nil
}

func (r *Repo) Freeze(path string, lock domain.PlanLock) error {
	locks, err := r.readLocks()
	if err != nil {
		return err
	}
	locks[path] = lock
	if err := os.MkdirAll(filepath.Dir(r.lockPath()), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(locks, "", "  ")
	return os.WriteFile(r.lockPath(), b, 0o644)
}
