package main

import "testing"

// docs/spec/auto-approval.md の binding の表（仕様からそのまま写した行）。

func TestAutoPolicy(t *testing.T) {
	assertRows(t, "auto-policy", []bindRow{
		{`null`, `{"enabled": false}`},
		{`{"auto_approve": {"llm_commands": []}}`, `{"enabled": false}`},
		{`{"auto_approve": {"enabled": true, "llm_commands": ["a"]}}`, `{"enabled": true, "llm_commands": ["a"], "runs": 3, "min_agree": 0, "kinds": ["example"], "require_reverse": true, "breaker": {"window": 20, "max_revoked": 2, "reset_at": ""}}`},
		{`{"auto_approve": {"enabled": true, "llm_commands": ["a", "b"], "runs": 2, "min_agree": 2, "kinds": ["example", "manual"], "require_reverse": false, "breaker": {"window": 5, "max_revoked": 1, "reset_at": "2026-11-04T00:00:00Z"}}}`, `{"enabled": true, "llm_commands": ["a", "b"], "runs": 2, "min_agree": 2, "kinds": ["example", "manual"], "require_reverse": false, "breaker": {"window": 5, "max_revoked": 1, "reset_at": "2026-11-04T00:00:00Z"}}`},
		{`{"auto_approve": {"enabled": true, "llm_commands": []}}`, `{"error": "llm_commands"}`},
		{`{"auto_approve": {"enabled": true, "llm_commands": ["a"], "runs": 1}}`, `{"error": "runs"}`},
		{`{"auto_approve": {"enabled": true, "llm_commands": ["a"], "runs": 11}}`, `{"error": "runs"}`},
		{`{"auto_approve": {"enabled": true, "llm_commands": ["a"], "runs": 3, "min_agree": 4}}`, `{"error": "min_agree"}`},
		{`{"auto_approve": {"enabled": true, "llm_commands": ["a"], "kinds": ["example", "foo"]}}`, `{"error": "kinds"}`},
		{`{"auto_approve": {"enabled": true, "llm_commands": [], "runs": 1}}`, `{"error": "llm_commands"}`},
	})
}

func TestAutoRunCommands(t *testing.T) {
	assertRows(t, "auto-run-commands", []bindRow{
		{`{"commands": ["a"], "runs": 3}`, `["a", "a", "a"]`},
		{`{"commands": ["a", "b"], "runs": 3}`, `["a", "b", "a"]`},
		{`{"commands": ["a", "b", "c", "d"], "runs": 2}`, `["a", "b"]`},
	})
}

func TestAutoEligible(t *testing.T) {
	assertRows(t, "auto-eligible", []bindRow{
		{`{"policy": {"enabled": true, "kinds": ["example"], "min_agree": 0, "require_reverse": true}, "paused": false, "candidate": {"kind": "example", "title": "円未満は切り捨て", "confidence": "3/3", "dry_run": "pass", "reverse": "match"}}`, `"ok"`},
		{`{"policy": {"enabled": false, "kinds": ["example"], "min_agree": 0, "require_reverse": true}, "paused": false, "candidate": {"kind": "example", "title": "円未満は切り捨て", "confidence": "3/3", "dry_run": "pass", "reverse": "match"}}`, `"disabled"`},
		{`{"policy": {"enabled": true, "kinds": ["example"], "min_agree": 0, "require_reverse": true}, "paused": true, "candidate": {"kind": "example", "title": "円未満は切り捨て", "confidence": "3/3", "dry_run": "pass", "reverse": "match"}}`, `"paused"`},
		{`{"policy": {"enabled": true, "kinds": ["example"], "min_agree": 0, "require_reverse": true}, "paused": false, "candidate": {"kind": "manual", "title": "締めは月末日", "confidence": "3/3", "dry_run": "none", "reverse": "match"}}`, `"kind"`},
		{`{"policy": {"enabled": true, "kinds": ["example"], "min_agree": 0, "require_reverse": true}, "paused": false, "candidate": {"kind": "example", "title": "円未満は切り捨て", "confidence": "2/3", "dry_run": "pass", "reverse": "match"}}`, `"agree"`},
		{`{"policy": {"enabled": true, "kinds": ["example"], "min_agree": 2, "require_reverse": true}, "paused": false, "candidate": {"kind": "example", "title": "円未満は切り捨て", "confidence": "2/3", "dry_run": "pass", "reverse": "match"}}`, `"ok"`},
		{`{"policy": {"enabled": true, "kinds": ["example"], "min_agree": 0, "require_reverse": true}, "paused": false, "candidate": {"kind": "example", "title": "端数は切り捨て【要確認: 負の数】", "confidence": "3/3", "dry_run": "pass", "reverse": "match"}}`, `"ambiguous"`},
		{`{"policy": {"enabled": true, "kinds": ["example"], "min_agree": 0, "require_reverse": true}, "paused": false, "candidate": {"kind": "example", "title": "円未満は切り捨て", "confidence": "3/3", "dry_run": "fail: 3 件中 1 件が期待と違う", "reverse": "match"}}`, `"dry-run"`},
		{`{"policy": {"enabled": true, "kinds": ["example"], "min_agree": 0, "require_reverse": true}, "paused": false, "candidate": {"kind": "example", "title": "円未満は切り捨て", "confidence": "3/3", "dry_run": "pass", "reverse": "mismatch"}}`, `"reverse"`},
		{`{"policy": {"enabled": true, "kinds": ["example"], "min_agree": 0, "require_reverse": false}, "paused": false, "candidate": {"kind": "example", "title": "円未満は切り捨て", "confidence": "3/3", "dry_run": "pass", "reverse": "skipped"}}`, `"ok"`},
	})
}

func TestAutoEvidence(t *testing.T) {
	assertRows(t, "auto-evidence", []bindRow{
		{`{"by": "nonchan7720", "evidence": null}`, `{"approved": true, "detail": ""}`},
		{`{"by": "auto", "evidence": {"sha_match": true, "verdict": "ok"}}`, `{"approved": true, "detail": ""}`},
		{`{"by": "auto", "evidence": null}`, `{"approved": false, "detail": "自動承認の証拠が無い"}`},
		{`{"by": "auto", "evidence": {"sha_match": false, "verdict": "ok"}}`, `{"approved": false, "detail": "証拠が承認時と違う"}`},
		{`{"by": "auto", "evidence": {"sha_match": true, "verdict": "reverse"}}`, `{"approved": false, "detail": "証拠の判定が ok でない"}`},
		{`{"by": "auto", "evidence": {"sha_match": false, "verdict": "reverse"}}`, `{"approved": false, "detail": "証拠が承認時と違う"}`},
	})
}

func TestAutoBreaker(t *testing.T) {
	assertRows(t, "auto-breaker", []bindRow{
		{`{"window": 20, "max_revoked": 2, "history": []}`, `false`},
		{`{"window": 20, "max_revoked": 2, "history": [{"revoked": true}, {"revoked": false}]}`, `false`},
		{`{"window": 20, "max_revoked": 2, "history": [{"revoked": true}, {"revoked": true}]}`, `true`},
		{`{"window": 2, "max_revoked": 2, "history": [{"revoked": true}, {"revoked": true}, {"revoked": false}]}`, `false`},
		{`{"window": 3, "max_revoked": 2, "history": [{"revoked": true}, {"revoked": true}, {"revoked": false}]}`, `true`},
	})
}

func TestGuard(t *testing.T) {
	assertRows(t, "guard", []bindRow{
		{`{"tool": "Bash", "command": "spec-gate auto-approve docs/spec/a.md#x --area auto", "agent_type": "spec-gate-approver"}`, `"allow"`},
		{`{"tool": "Bash", "command": "spec-gate auto-approve docs/spec/a.md#x --area auto"}`, `"deny"`},
		{`{"tool": "Bash", "command": "go run . auto-approve docs/spec/a.md#x --area auto", "agent_type": "general-purpose"}`, `"deny"`},
		{`{"tool": "Bash", "command": "spec-gate audit --since 24h", "agent_type": "spec-gate-approver"}`, `"allow"`},
		{`{"tool": "Bash", "command": "spec-gate audit --since 24h"}`, `"allow"`},
		{`{"tool": "Bash", "command": "spec-gate audit; rm -rf claims", "agent_type": "spec-gate-approver"}`, `"deny"`},
		{`{"tool": "Bash", "command": "ls", "agent_type": "spec-gate-approver"}`, `"deny"`},
		{`{"tool": "Edit", "path": "pkg/domain/claim.go", "agent_type": "spec-gate-approver"}`, `"deny"`},
		{`{"tool": "Bash", "command": "spec-gate revoke AUTO-001 --reason x"}`, `"deny"`},
		{`{"tool": "Bash", "command": "go test ./..."}`, `"allow"`},
		{`{"tool": "Edit", "path": ".claude/gate.yaml"}`, `"deny"`},
		{`{"tool": "Write", "path": ".claude/feedback/protect_spec_claims.md"}`, `"deny"`},
		{`{"tool": "Edit", "path": ".claude/agents/spec-gate-approver.md"}`, `"deny"`},
		{`{"tool": "Edit", "path": ".claude/agents/spec-gate-llm-worker.md"}`, `"allow"`},
		{`{"tool": "Edit", "path": ".claude/settings.json"}`, `"deny"`},
		{`{"tool": "Write", "path": ".spec-gate/policy.yaml"}`, `"deny"`},
		{`{"tool": "Edit", "path": "pkg/domain/claim.go"}`, `"allow"`},
		{`{"tool": "Bash", "command": "echo x > .claude/gate.yaml"}`, `"deny"`},
		{`{"tool": "Bash", "command": "cat .claude/gate.yaml"}`, `"allow"`},
	})
}

// 「固定版で動かす」の表。
func TestGuardPinned(t *testing.T) {
	assertRows(t, "guard", []bindRow{
		{`{"tool": "Edit", "path": "bin/spec-gate"}`, `"deny"`},
		{`{"tool": "Write", "path": "bin/.pinned/v0.0.0-dev.948bc6c/spec-gate"}`, `"deny"`},
		{`{"tool": "Edit", "path": ".mcp.json"}`, `"deny"`},
		{`{"tool": "Write", "path": "scripts/mcp.sh"}`, `"deny"`},
		{`{"tool": "Edit", "path": "scripts/pin-spec-gate.sh"}`, `"allow"`},
		{`{"tool": "Bash", "command": "go build -o bin/spec-gate ."}`, `"deny"`},
		{`{"tool": "Bash", "command": "go build -o bin/spec-gate.real ."}`, `"allow"`},
		{`{"tool": "Bash", "command": "ln -sf /tmp/x bin/spec-gate"}`, `"deny"`},
		{`{"tool": "Bash", "command": "rm -rf bin/.pinned"}`, `"deny"`},
	})
}
