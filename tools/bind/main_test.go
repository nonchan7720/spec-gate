package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestRun_CallsRegisteredFunction(t *testing.T) {
	r := registry{"double": func(in json.RawMessage) (any, error) {
		var x float64
		if err := json.Unmarshal(in, &x); err != nil {
			return nil, err
		}
		return x * 2, nil
	}}
	var out bytes.Buffer
	if err := r.run("double", strings.NewReader(`{"in": 21}`), &out); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != `{"out":42}` {
		t.Fatalf("out = %q", out.String())
	}
}

func TestRun_NullOutputIsKept(t *testing.T) {
	r := registry{"none": func(json.RawMessage) (any, error) { return nil, nil }}
	var out bytes.Buffer
	if err := r.run("none", strings.NewReader(`{"in": 1}`), &out); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != `{"out":null}` {
		t.Fatalf("null は null として返す（期待値 null と比べるため）: %q", out.String())
	}
}

func TestRegister_AddsAndRejectsDuplicates(t *testing.T) {
	saved := funcs
	funcs = registry{}
	t.Cleanup(func() { funcs = saved })

	register("x", func(json.RawMessage) (any, error) { return 1, nil })
	if _, ok := funcs["x"]; !ok {
		t.Fatal("register した関数が引ける")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("同じ名前の二重登録は起動時に落とす（機能ごとのファイルで名前がぶつかったのを見逃さない）")
		}
	}()
	register("x", func(json.RawMessage) (any, error) { return 2, nil })
}

func TestRun_Errors(t *testing.T) {
	r := registry{"boom": func(json.RawMessage) (any, error) { return nil, errors.New("boom") }}
	cases := map[string]struct {
		name, stdin, want string
	}{
		"未登録の名前":   {"nope", `{"in": 1}`, "nope"},
		"壊れた JSON": {"boom", `{`, "stdin"},
		"関数のエラー":   {"boom", `{"in": 1}`, "boom"},
	}
	for label, c := range cases {
		t.Run(label, func(t *testing.T) {
			var out bytes.Buffer
			err := r.run(c.name, strings.NewReader(c.stdin), &out)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want containing %q", err, c.want)
			}
			if out.Len() != 0 {
				t.Fatalf("失敗時は stdout に何も出さない: %q", out.String())
			}
		})
	}
}
