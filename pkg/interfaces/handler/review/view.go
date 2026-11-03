package review

import (
	"sort"
	"strings"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/services"
)

// itemView は画面に出す候補 1 件。YAML は出さず、種類ごとに読める形へ組み替える。
type itemView struct {
	Name       string     `json:"name"`
	Path       string     `json:"path,omitempty"`
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	Title      string     `json:"title"`
	Confidence string     `json:"confidence,omitempty"`
	Area       string     `json:"area,omitempty"`
	Error      string     `json:"error,omitempty"`
	Source     sourceView `json:"source"`
	Spec       specView   `json:"spec"`

	// kind: example
	Binding    string        `json:"binding,omitempty"`
	Examples   []exampleView `json:"examples,omitempty"`
	Properties []string      `json:"properties,omitempty"`

	// kind: structure
	Layers      []layerView `json:"layers,omitempty"`
	FromDiagram bool        `json:"fromDiagram,omitempty"`
	Forbidden   []edgeView  `json:"forbidden,omitempty"`

	// kind: ui
	Story   string   `json:"story,omitempty"`
	States  []string `json:"states,omitempty"`
	URL     string   `json:"url,omitempty"`
	Strings []string `json:"strings,omitempty"`
}

type sourceView struct {
	Type     string `json:"type"`
	Ref      string `json:"ref"`
	Optional bool   `json:"optional,omitempty"`
}

type specView struct {
	Excerpt  string `json:"excerpt,omitempty"`
	Location string `json:"location,omitempty"`
	Error    string `json:"error,omitempty"`
}

type exampleView struct {
	In   string `json:"in"`
	Out  string `json:"out"`
	Note string `json:"note,omitempty"`
}

type layerView struct {
	Name string `json:"name"`
	Dir  string `json:"dir"`
}

type edgeView struct {
	From string `json:"from"`
	To   string `json:"to"`
	// Raw は "a -> b" の形に読めなかったときの元の文字列
	Raw string `json:"raw,omitempty"`
}

func toView(it services.ReviewItem) itemView {
	c := it.Claim
	v := itemView{
		Name: it.Name, Path: it.Path, ID: c.ID, Kind: string(c.Kind), Title: c.Title,
		Confidence: it.Confidence, Area: it.Area, Error: it.Error,
		Source:     sourceView{Type: string(c.Source.Type), Ref: c.Source.Ref, Optional: c.Source.Ref != "" && !c.Source.DriftRequired()},
		Spec:       specView{Excerpt: it.Spec.Excerpt, Location: it.Spec.Location, Error: it.SpecError},
		Binding:    c.Binding,
		Properties: c.Properties,
		Story:      c.Story, States: c.States, URL: c.URL, Strings: c.Strings,
		FromDiagram: c.Allowed == "from-diagram",
	}
	for _, ex := range c.Examples {
		v.Examples = append(v.Examples, exampleView{In: domain.FormatExampleInput(ex.In), Out: domain.FormatExampleValue(ex.Out), Note: ex.Note})
	}
	for name, dir := range c.Layers {
		v.Layers = append(v.Layers, layerView{Name: name, Dir: dir})
	}
	sort.Slice(v.Layers, func(i, j int) bool { return v.Layers[i].Name < v.Layers[j].Name })
	for _, f := range c.Forbidden {
		from, to, ok := strings.Cut(f, "->")
		if !ok {
			v.Forbidden = append(v.Forbidden, edgeView{Raw: f})
			continue
		}
		v.Forbidden = append(v.Forbidden, edgeView{From: strings.TrimSpace(from), To: strings.TrimSpace(to)})
	}
	return v
}
