package main

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// 仕様「LLM 処理のサブエージェント委譲」の binding 表。
func TestLLMPaths_SpecTable(t *testing.T) {
	cases := []struct{ in, want string }{
		{`{"ref": "docs/spec/rounding.md#端数処理", "runs": 2}`,
			`[".spec-gate-llm/extract/docs_spec_rounding.md_端数処理/run-1.prompt.md", ".spec-gate-llm/extract/docs_spec_rounding.md_端数処理/run-2.prompt.md"]`},
		{`{"ref": "docs/arch/layers.mmd", "runs": 1}`,
			`[".spec-gate-llm/extract/docs_arch_layers.mmd/run-1.prompt.md"]`},
	}
	for _, c := range cases {
		var out bytes.Buffer
		if err := funcs.run("llm-paths", strings.NewReader(`{"in": `+c.in+`}`), &out); err != nil {
			t.Fatal(err)
		}
		var got struct{ Out any }
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		var want any
		if err := json.Unmarshal([]byte(c.want), &want); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got.Out, want) {
			t.Errorf("in %s: out = %v, want %v", c.in, got.Out, want)
		}
	}
}
