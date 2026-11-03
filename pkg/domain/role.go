package domain

import "fmt"

// Role は check の結果を見る人の役割。役割ごとに対応すべきものだけを出す。
type Role string

const (
	RoleEngineer Role = "engineer" // fail（コードを直す）
	RoleDesigner Role = "designer" // kind: ui の unverified と fail（画面を確かめる）
	RolePM       Role = "pm"       // stale と unapproved（仕様と承認を決める）
)

// Roles は指定できる役割。
var Roles = []Role{RoleEngineer, RoleDesigner, RolePM}

// ParseRole は文字列を役割にする。知らない役割はエラー。
func ParseRole(s string) (Role, error) {
	for _, r := range Roles {
		if string(r) == s {
			return r, nil
		}
	}
	return "", fmt.Errorf("役割 %q は不明（engineer / designer / pm）", s)
}

// Wants はその役割が対応すべき判定か。
func (r Role) Wants(res Result) bool {
	switch r {
	case RoleEngineer:
		return res.Status == StatusFail
	case RoleDesigner:
		return res.Kind == KindUI && (res.Status == StatusUnverified || res.Status == StatusFail)
	case RolePM:
		return res.Status == StatusStale || res.Status == StatusUnapproved
	}
	return false
}

// FilterByRole は役割が対応すべき判定だけを、入力の順のまま返す。
func FilterByRole(role Role, rs Results) Results {
	out := Results{}
	for _, r := range rs {
		if role.Wants(r) {
			out = append(out, r)
		}
	}
	return out
}
