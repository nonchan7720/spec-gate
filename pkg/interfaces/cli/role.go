package cli

import (
	"fmt"
	"io"

	"github.com/nonchan7720/spec-gate/pkg/domain"
	"github.com/nonchan7720/spec-gate/pkg/services"
)

// roleLabel は役割ごとに何を出しているかの説明。
var roleLabel = map[domain.Role]string{
	domain.RoleEngineer: "fail（仕様の位置つき）",
	domain.RoleDesigner: "画面（kind: ui）の unverified と fail",
	domain.RolePM:       "stale と unapproved",
}

// filterReport は役割が対応すべき判定だけを残す。プラン項目にも同じ規則を当て、空になったプランは落とす。
func filterReport(rep services.CheckReport, role domain.Role) services.CheckReport {
	out := services.CheckReport{Claims: domain.FilterByRole(role, rep.Claims), Skipped: rep.Skipped}
	for _, p := range rep.Plans {
		items := domain.FilterByRole(role, p.Items)
		if len(items) == 0 {
			continue
		}
		out.Plans = append(out.Plans, services.PlanReport{Path: p.Path, Title: p.Title, Items: items})
	}
	return out
}

// writeRoleReport は役割が対応すべき判定だけを出す。
// engineer はコードを直す人なので、仕様の位置が分からないものも「不明」と明示する。
func writeRoleReport(w io.Writer, rep services.CheckReport, role domain.Role) {
	shown := filterReport(rep, role)
	_, _ = fmt.Fprintf(w, "## %s: %s\n", role, roleLabel[role])
	n := 0
	for _, r := range shown.Claims {
		writeRoleResult(w, r, role, "")
		n++
	}
	for _, p := range shown.Plans {
		for _, r := range p.Items {
			// プラン項目は仕様ではなくプランファイルが根拠
			writeRoleResult(w, r, role, p.Path)
			n++
		}
	}
	_, _ = fmt.Fprintf(w, "\n%s が対応すべきもの: %d 件\n", role, n)
}

func writeRoleResult(w io.Writer, r domain.Result, role domain.Role, fallbackSpec string) {
	if role == domain.RoleEngineer && r.Spec == "" {
		r.Spec = fallbackSpec
		if r.Spec == "" {
			r.Spec = "仕様の位置不明"
		}
	}
	writeResult(w, r, false)
}
