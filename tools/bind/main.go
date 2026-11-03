// Command bind は example claim の binding 用ディスパッチャ（Go 関数向け）。
//
//	go run ./tools/bind <name>   stdin: {"in": ...}  stdout: {"out": ...}
//
// 関数は機能ごとのファイル（例: leak.go）の init で register して足す。main.go は触らない。
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// fn は {"in": ...} の中身を受け取り、{"out": ...} に入れる値を返す。
type fn func(in json.RawMessage) (any, error)

type registry map[string]fn

var funcs = registry{}

// register は binding 名に関数を登録する。名前の重複は起動時に落とす。
func register(name string, f fn) {
	if _, dup := funcs[name]; dup {
		panic("tools/bind: duplicate binding " + name)
	}
	funcs[name] = f
}

func (r registry) run(name string, stdin io.Reader, stdout io.Writer) error {
	f, ok := r[name]
	if !ok {
		names := make([]string, 0, len(r))
		for n := range r {
			names = append(names, n)
		}
		sort.Strings(names)
		return fmt.Errorf("binding %q は未登録（登録済み: %s）", name, strings.Join(names, ", "))
	}
	var req struct {
		In json.RawMessage `json:"in"`
	}
	if err := json.NewDecoder(stdin).Decode(&req); err != nil {
		return fmt.Errorf("stdin の JSON が読めない: %w", err)
	}
	out, err := f(req.In)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	b, err := json.Marshal(map[string]any{"out": out})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, string(b))
	return err
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./tools/bind <name>")
		os.Exit(2)
	}
	if err := funcs.run(os.Args[1], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
