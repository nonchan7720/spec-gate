English | [日本語](README_ja.md)

# spec-gate — a spec-compliance harness

```
Project coding rules         → hook-gate
Keep code debt from growing  → depgraph
Guarantee product quality    → spec-gate ← this
```

spec-gate decides, by exit code and without ever trusting the agent's own report, whether
**code written by an AI honours the natural-language specs** (Markdown / Mermaid / Figma / images)
and the plan files made together with the agent.

One principle: **assume the AI lies, and move the verdict to machines. As models get better, the
harness simply lets them through and the round trips shrink.**

## How it works

```
source (anything)  →  adapter  →  claim (structured)  →  check (exit code)
Markdown / Mermaid /              one YAML per claim       example / structure / ui / plan
Figma / PNG / PDF                 ↑ the only human approval
```

Natural language is never compared with code directly. A **claim (a verifiable statement)** sits in between.

| Step | Command | Who | What happens |
|---|---|---|---|
| Extract | `spec-gate extract <ref> --runs 3 --emit` / `--collect` | AI (subagents) | The same spec is read N times by independent subagents; agreed claims go to `claims/_pending/` with high confidence (consensus). See [Delegating extract and reverse](#delegating-extract-and-reverse-to-subagents) |
| Lint | `spec-gate lint` | machine | Do examples satisfy properties; are holdouts well-formed; do claims contradict each other |
| Approve | `spec-gate approve <id>` | **human** | Shows the spec excerpt and freezes its hash. Refused when lint fails |
| Review | `spec-gate review` | **human** | A local web page (127.0.0.1) listing `claims/_pending/` in plain language with the spec excerpt; approve / reject / ambiguous per candidate. Approving moves the claim into `claims/<area>/` and runs `approve` |
| Review UI (remote) | `spec-gate mcp` (started from `.mcp.json`) | **human** | An MCP Apps screen that also opens in remote sessions (claude.ai/code, the Claude mobile app). Candidates are sorted by how easy they are to approve; approve / reject / ambiguous / bulk approve by tapping. See [Review UI (MCP Apps)](#review-ui-mcp-apps) |
| Remote approval | `spec-gate review ask` → question dialog | **human** answers | The same three buttons in Claude Code's question dialog, for remote sessions. See [Remote approval](#remote-approval-claude-codes-question-dialog) |
| Check | `spec-gate check` | machine | approval → spec drift → per-kind check (examples, holdout, fuzz) → plan verify |
| Coverage | `spec-gate coverage` | machine | Lists the parts of the specs no claim covers, and the parts marked ambiguous ("to confirm") |
| Reverse | `spec-gate reverse [<id>...] --emit` / `--collect` | AI (subagents) | Rewrites each claim as spec prose without seeing the spec, then compares it with the original excerpt and lists what the claim adds, misses or contradicts (review aid; `--strict` exits 1 on a mismatch) |

### Order of the verdict and states

`check` looks at each claim in this order and stops at the first stage that catches it.

```
unapproved  no approval                                    → blocking
stale       the spec changed after approval                → blocking (until a human re-approves)
fail        the code does not satisfy the claim            → blocking (the agent fixes it)
unverified  no means to verify (manual / no --ui / depgraph could not decide) → for human review; does not stop the agent
pass
```

Any blocking result exits 1. With `--hook` it writes a report of only what the agent must fix to stderr and exits 2.

For `kind: example`, "fail" covers the examples, the [holdout](#holdout-examples-hidden-from-the-agent) and the
[random inputs](#fuzz-and-relations--random-inputs) alike. When a Markdown claim's heading is gone from the spec
(`source.ref` points to a heading that no longer exists), the claim fails with "spec cannot be read", and the
detail suggests the closest heading as "もしかして" (did you mean). Closeness is the character edit distance, only
headings within half the missing heading's length are suggested, and ties go to the heading that appears first.
It is only a suggestion: the claim is not rewritten (fixing it is a human's job).

### Who does what

| Human only | Agent (or anyone) |
|---|---|
| `approve`, `review` (approve / reject / ambiguous in the browser) | `status`, `lint`, `check`, `coverage`, `metrics`, `trace`, `leak` |
| answering the remote-approval dialog | `review ask` (printing the questions for the dialog) |
| `plan freeze` | writing plans with `verify:` lines, `plan check`, `plan mutate` |
| `holdout add` | `extract` / `reverse` with `--emit` / `--collect` (the LLM work goes to subagents) |
| editing `claims/**` (except `_pending/`), `.spec-gate/`, `holdout/`, `.claude/settings*.json` | putting a proposed fix for a wrong claim in `claims/_pending/` and telling a human |

The human-only column is enforced, not just asked for: hook-gate rules (`.claude/feedback/`) deny the agent
running those commands and editing those paths, and `review record` (the remote-approval hook) is started only by
the harness.

### A structure that cannot lie

- **The AI cannot touch claims.** Editing `claims/**` and `.spec-gate/`, and running `approve` / `plan freeze`,
  are denied by hook-gate `pre_edit` / `pre_bash` rules (`.claude/feedback/protect_spec_claims.md`). The same file
  denies running `review record` directly and calling the review page's `/api/approve`, `/api/reject` and
  `/api/ambiguous` from the shell
- **Plan verify lines are frozen first.** `spec-gate plan freeze` records the hash of the verify lines; loosening
  them after implementation makes the plan `stale`. On top of that, right after a plan is edited, a hook-gate
  `post_edit` rule runs `spec-gate plan check` and stops the agent on the spot when a frozen verify line changed
  (`.claude/feedback/protect_plan_verify.md`). A plan can also demand that its verify lines catch broken code
  before it may be frozen ([mutation score](#plan-mutate-and-the-mutation-threshold))
- **A changed spec becomes stale automatically**, so the spec cannot be rewritten to make things fit
- **A claim without a means of verification is unverified, not fail.** Nothing "passes without being checked";
  instead it is surfaced as something a human should look at
- **Passing only the examples is not enough.** Holdout examples the agent cannot read, random inputs, a warning
  when example values are written into the implementation, and a log of how each claim came to pass close the
  usual shortcuts (see [Closing the shortcuts](#closing-the-shortcuts))

## Install

**GitHub Releases (recommended).** Download the archive for your OS / CPU from
[Releases](https://github.com/nonchan7720/spec-gate/releases): `spec-gate_<os>_<arch>.tar.gz`
(`.zip` on Windows) for linux / darwin / windows × amd64 / arm64. Verify it with `checksums.txt`,
then put `spec-gate` on your `PATH`. Every archive also contains `LICENSE`, `NOTICE` and
`THIRD_PARTY_NOTICES.md`.

```bash
sha256sum --ignore-missing -c checksums.txt   # macOS: shasum -a 256 --ignore-missing -c checksums.txt
tar xzf spec-gate_linux_amd64.tar.gz && install spec-gate ~/.local/bin/
spec-gate --version
```

Releases named `v<version>-dev.<commit>` are prereleases built from every push to `main`.

**With Go.**

```bash
go install github.com/nonchan7720/spec-gate@latest
```

**From source.** Clone the repository and build with the toolchain pinned in `mise.toml`:

```bash
git clone https://github.com/nonchan7720/spec-gate.git && cd spec-gate
mise run setup   # mise install, go mod download, the sample app's npm packages, bin/spec-gate
```

spec-gate is pure Go (`CGO_ENABLED=0`), so it cross-compiles with plain `go build`.

## Run it

```sh
go build -o spec-gate .                    # or: go run .
pnpm install --frozen-lockfile             # the sample TS side (for tools/call.ts)

./spec-gate status                         # list claims and plans
./spec-gate lint                           # internal consistency
./spec-gate approve ROUND-001 ROUND-002 CLOSE-001 ARCH-001 UI-001   # human
./spec-gate check                          # full verdict
./spec-gate check --changed                # only the claims related to changed files
./spec-gate coverage                       # what is missing / what is ambiguous
./spec-gate plan check plans/x.md          # has a frozen plan's verify changed
./spec-gate plan mutate plans/x.md         # how many injected bugs the verify lines catch
./spec-gate leak src/payroll/round.ts      # are example values written into the code
./spec-gate metrics                        # claims per kind and the unverified rate
./spec-gate trace                          # how each claim came to pass
```

The sample ships already approved, so `check` passes right after cloning. Breaking it shows the idea:

```sh
sed -i 's/Math.floor/Math.round/' src/payroll/round.ts && ./spec-gate check --only ROUND-001
#  ✗ ROUND-001  fail  円未満は切り捨て  [docs/spec/rounding.md:10]
#        examples[0]: in=1234.56 期待=1234 実際=1235

sed -i '/infra: \[app\]/d' .depgraph.yml && ./spec-gate check --only ARCH-001
#  ✗ ARCH-001  fail  (the diagram and .depgraph.yml disagree)
#        infra -> app is not allowed by the spec but forbidden.infra does not list app

sed -i 's/切り捨てる/四捨五入する/' docs/spec/rounding.md && ./spec-gate check --only ROUND-001
#  ⟳ ROUND-001  stale  (the spec changed after approval; review it and re-approve)
```

(The sample specs and messages are in Japanese.)

### Connecting to Claude Code

The checks run from hook-gate (feedback-gate) through `.claude/gate.yaml`. spec-gate has no Stop hook of its own.

- On every edit, depending on the changed file: gofmt / go vet / go test (Go), `depgraph gate` (Go / TS / JS),
  `spec-gate lint` (claims), `spec-gate check --changed` (specs, plans, the sample app)
- After editing an implementation file (`.go` / `.ts` / `.js` and friends; tests, `claims/`, `holdout/` and `docs/`
  excluded), the `post_edit` rule in `.claude/feedback/detect_literal_leak.md` runs `spec-gate leak` on it and warns
- At stop, the `consistency_checks` `go-all` (build / golangci-lint / all tests) and `spec-gate`
  (`check --changed --hook`) run and block the stop on failure. Giving up after repeated failures (5 times)
  is hook-gate's job

depgraph uses the Neo4j started by `docker compose up -d` (`localhost:17687`); the connection is set in `[env]` of
`mise.toml`, and depgraph is always called through `mise exec --` (hooks and gate). The SessionStart hook in `.claude/settings.json`
(`depgraph hook session-start`) records the baseline at session start, and `depgraph gate` fails only on debt added since then.

`CLAUDE.md` holds the rules for the agent. The subagent definition and skills for extract / reverse ship in
`.claude/agents/` and `.claude/skills/` (next section).

### Delegating extract and reverse to subagents

spec-gate itself never calls an LLM. `extract` (consensus extraction) and `reverse` (reverse check) only **write
prompts** (`--emit`) and **read answers** (`--collect`); the agent running spec-gate hands each prompt to a fresh
subagent.

- `extract <ref> --runs N --emit` writes N self-contained prompts under `.spec-gate-llm/extract/<ref>/run-<k>.prompt.md`
  and prints their paths. `extract <ref> --collect` reads the N answers (`run-<k>.answer.md`), runs the same consensus
  as before and puts the result in `claims/_pending/`. Missing answers are listed and the command fails
- `reverse [<id>...] --emit` writes one restatement prompt per claim. `reverse --collect` then writes the comparison
  prompts once all restatements are in, and prints the report once all comparisons are in (`--strict` exits 1 on a
  mismatch or error)
- `.spec-gate-llm/` is git-ignored and removed when collect finishes (it is not under `.spec-gate/`, which the agent
  may not write)
- `.claude/agents/spec-gate-llm-worker.md` is a single-use worker with only Read / Write: it reads its one prompt
  file and writes its one answer file, nothing else. The skills `spec-gate-extract` and `spec-gate-reverse`
  (`.claude/skills/`) describe the steps: emit → start one new worker per prompt, all in parallel, with only the
  message `Read <prompt path> and follow it.` → collect. Never reuse a worker and never add hints, so the runs stay
  independent

Only when `SPEC_GATE_EXTRACT_CMD` / `SPEC_GATE_REVERSE_CMD` is set does spec-gate call that command (for example
`claude -p`; prompt on stdin, answer on stdout), as it used to. Without the variable and without `--emit` /
`--collect`, the command prints how to delegate and exits 1, so no LLM is called by accident.

```sh
spec-gate extract docs/spec/rounding.md#端数処理 --runs 3 --emit
#   (one spec-gate-llm-worker per printed prompt)
spec-gate extract docs/spec/rounding.md#端数処理 --collect

SPEC_GATE_EXTRACT_CMD='claude -p' spec-gate extract --type mermaid docs/arch/layers.mmd --runs 5
```

## Layout

```
docs/spec/*.md        detailed specs (Markdown)        ─┐
docs/arch/*.mmd       architecture diagrams (Mermaid)   ├ sources
Figma / *.png / *.pdf  UI and others                   ─┘
sources.yaml          registry of watched sources (for coverage)
claims/<area>/*.yaml  claims. Format: claims/SCHEMA.md
claims/_pending/      extract output. Not checked until approved
claims/_ambiguous/    candidates recorded as "the spec is not decided". Not checked; shown by coverage
holdout/<id>.yaml     holdout examples (inputs + hashes of the expected outputs). Written by humans only
plans/*.md            plans. Checked when the front matter says status: active
.spec-gate/           plan locks (committed); trace.jsonl and review-decisions.jsonl (git-ignored)
.spec-gate-llm/       prompts and answers while extract / reverse are delegated (git-ignored, temporary)
.depgraph.yml         depgraph's layer definition. The structure claim checks it matches the diagram
.claude/gate.yaml     hook-gate quality gate
.claude/feedback/     hook-gate rules (protect claims, verify lines and holdout; literal-leak warning)
.claude/agents/       spec-gate-llm-worker (subagent for extract / reverse)
.claude/skills/       spec-gate-extract / spec-gate-reverse (the delegation steps)
tools/call.ts         binding wrapper for example claims (TS functions)
tools/bind            bindings for the examples in docs/spec/harness.md (spec-gate's own spec)

main.go               entry point
pkg/domain            Claim / Plan / Result, ports, lint, fuzz, mutation, leak, relink (pure)
pkg/services          check / approve / coverage / extract / reverse / plan / mutate / holdout / metrics / trace / review
pkg/infrastructure    claimrepo, adapter(markdown/mermaid/figma/image), checker, planfile, vcs, extractor, reverser, llmwork, tracelog, workspace
pkg/interfaces        cli (cobra commands, reports, hook output), handler/review (the review page)
```

## Claim kinds and what can be enforced mechanically

| kind | example source | verification | confidence |
|---|---|---|---|
| `example` | detailed specs (rounding, closing dates) | Feeds input/output examples (and holdout inputs) to the binding command and evaluates properties (expr); optionally random inputs and metamorphic relations | high, deterministic |
| `structure` | Mermaid diagram | Checks that the diagram's edges (= allowed) agree with `layers` / `forbidden` in `.depgraph.yml`, then has `depgraph scan` detect violations | the comparison is deterministic; detection depends on depgraph |
| `ui` | Figma | A story export with the same name as each variant / with `--ui`, Playwright checks the strings | medium |
| `manual` | anything | none; always unverified | — |

The `example` binding is any command honouring "stdin JSON → stdout JSON", so the same claim verifies TS, Go or Python.

### fuzz and relations — random inputs

To catch implementations that satisfy only the handful of examples, an `example` claim can have `fuzz:` and
`relations:` (format: `claims/SCHEMA.md`).

```yaml
fuzz:
  runs: 200            # 1–10000 generated inputs
  seed: 42             # optional; defaults to the 64-bit FNV-1a hash of the claim ID
  input: { type: number, min: -100000, max: 100000 }   # number | integer; both ends inclusive
relations:             # metamorphic relations; x = the generated input, f(...) = a binding call
  - "f(x + 1) == f(x) + 1"
```

After all examples match, `check` generates `runs` inputs deterministically from the seed (same seed, same inputs),
feeds them to the binding, and evaluates the properties (`input` = x, `output` = the result) and the relations for
each x. The first input that breaks one makes the claim fail with the counterexample and the seed, so writing that
`seed:` reproduces it. `lint` requires fuzz for relations, properties or relations for fuzz, and relations that
return a boolean.

### Holdout examples hidden from the agent

A holdout is a set of input/output examples the agent never sees, kept apart from the claim's examples in
`holdout/<claim-id>.yaml`. An agent with a shell can read anything you can read, so a plain-text holdout could not
stay hidden; instead **the expected output is stored only as a hash**:

```yaml
examples:
  - in: 98765.43
    out_sha256: 3f1c…   # SHA-256 of the canonical JSON of the expected output (64 lowercase hex chars)
```

- `check` feeds the holdout inputs to the binding after the examples and compares the SHA-256 of the canonical JSON
  of the output with `out_sha256`. Properties are evaluated on the holdout input and the actual output. Any
  mismatch fails the claim, and reports show **only the count** ("holdout 2 件中 1 件が不一致（値は表示しない）"),
  never the input, the expected value or the actual value
- A plain `out`, a missing `in` / `out_sha256` or a malformed hash is rejected when the file is read. `lint` checks
  only the shape (the claim exists and is `kind: example`)
- **Only humans add holdouts**: `spec-gate holdout add <claim-id> --in <json> --out <json>` appends `in` and
  `out_sha256`, creating the file if needed; the plain `out` is neither stored nor printed. (It still passes through
  your shell's command line, so mind your shell history)
- The agent can neither read nor write `holdout/`: `.claude/settings.json` has `permissions.deny` for
  `Read` / `Edit` / `Write(./holdout/**)`, and hook-gate denies shell commands that read or write it
  (`.claude/feedback/protect_holdout.md`) and running `holdout add` (`.claude/feedback/protect_holdout_add.md`).
  Only spec-gate itself (`check` / `lint`) reads it

### structure and depgraph

Diagram = spec, depgraph = detector, spec-gate = agreement between the two.

1. Every layer of the claim exists under the same name in `layers` of `.depgraph.yml` and points to the same directory (`<dir>/**`)
2. For every ordered pair A→B of the claim's layers: a dependency the diagram allows (minus the claim's `forbidden`)
   is not forbidden by depgraph, and one it does not allow is forbidden by depgraph
3. When they agree, `depgraph scan --root . --claude-md ''` (override with `SPEC_GATE_DEPGRAPH_CMD`) scans the current tree;
   any `layer-violation` between two of the claim's layers is fail, including violations that existed before the session
   (CLAUDE.md is left untouched). When depgraph is not installed, cannot reach Neo4j or does not finish the scan, the result is unverified

### Source adapters

| type | ref | what is hashed | coverage unit |
|---|---|---|---|
| markdown | `path.md#heading` | the section body (whitespace normalised) | heading |
| mermaid | `path.mmd` | the edge set (labels and layout ignored) | edge |
| figma | `figma://<fileKey>/<nodeId>` | the node subtree (coordinates excluded). Needs `FIGMA_TOKEN` | text node, variant |
| image | `path.png` | the file hash | file |

For sources the AI cannot read directly, such as Figma or images, `extract` accuracy is not guaranteed.
Since a human approves, low-quality candidates cannot slip mistakes in.

### What `check --changed` re-checks

`check --changed` judges only the claims related to the files changed in git, leaning towards checking too much
rather than missing something. A claim touches its spec file, its claim file, its `story`, its `layers`, and every
word of its `binding` that names a file or directory that actually exists in the repository. A changed file is
related when it is

- a touched file itself, or under a touched directory
- in the same directory as a touched file and with the same extension (another file of the same package)
- in a Go package that a touched Go package imports, directly or indirectly, within this module (so a binding like
  `go run ./tools/bind relink` is re-checked when the implementation it calls changes). When the imports cannot be
  resolved, nothing is added and the original paths stay
- `holdout/<claim-id>.yaml` of that claim (when a human adds a holdout)

## Closing the shortcuts

An agent may try to satisfy a claim by writing the example values into the code, by passing only the few examples,
or by weakening the checks. These features block that mechanically, or at least make it visible to a human
(spec: `docs/spec/harness.md`).

| Shortcut | What catches it |
|---|---|
| passing only the examples | [holdout](#holdout-examples-hidden-from-the-agent), [fuzz / relations](#fuzz-and-relations--random-inputs) |
| writing example values into the code | `spec-gate leak` |
| weak verify lines in a plan | `plan mutate` and the `mutation:` threshold |
| forcing a claim through after many failures | `spec-gate trace` |

### `leak` — example values written into the implementation

`spec-gate leak <file>...` looks for the claims' example `in` / `out` values and the holdout `in` values in the given
files and exits 1 with the value and line number when one appears (for values found only in a holdout, only the
line number). Numbers count only with 3 or more digits (sign and decimal point excluded) and only as a whole token
(the `1234` inside `1234.56` does not count); strings count from 4 characters. Test files (`_test.go`, `.test.ts`,
`.spec.ts`) and `claims/`, `holdout/`, `docs/` are skipped. hook-gate runs it after each edit of an implementation
file and warns.

### `plan mutate` and the mutation threshold

`spec-gate plan mutate <plans/x.md>` injects one mutation at a time into the files the plan's claims touch (binding
paths, layers, story), runs the plan's verify lines, and prints the share of mutations they catch (mutation score)
and the ones that survived. The mutations are: arithmetic `+`↔`-`, `*`↔`/`; comparisons `>` → `>=` / `<`,
`<` → `<=` / `>`, `>=` → `>`, `<=` → `<`, `==`↔`!=`; an integer literal n ≥ 2 → n+1 (never inside strings or
comments). The verify lines must pass before any mutation; at most `--max` mutations (default 50) are picked evenly.
The files are always restored, whether the run succeeds, fails or is interrupted (Ctrl-C).

With a threshold in the plan's front matter, `plan freeze` (human) runs the same score first and refuses to freeze
below it, or when no mutation can be made at all:

```yaml
---
status: draft
claims: [ROUND-001]
mutation: 0.8   # 0–1
---
```

`plan mutate` exits 1 when the plan has a threshold that is not met. The agent can run it to strengthen the verify
lines before asking a human to freeze.

### `trace` — how a claim came to pass

Every `check --hook` appends one line per claim to `.spec-gate/trace.jsonl` (time, claim id, status, and the number
of changed lines = added + deleted in `git diff --numstat HEAD`, 0 outside git; the file is git-ignored).
`spec-gate trace` (`--json` for JSON) shows, for the last time each claim passed, how many failures came right
before it and how many lines were changed at that point. A claim that failed many times and then passed with a large
change is worth a human look.

## CI output and role reports

- `check --json` prints the verdict as JSON: `id`, `kind`, `status`, `title`, `detail`, `spec` (file:line) per claim,
  plus the plans and the counts. The exit code is the same as plain `check` (1 when anything blocks)
- `spec-gate metrics` judges every claim like `check` (no plans, no `--ui`) and prints the count and share per kind and
  the unverified rate (unverified / all claims; 0 when there are no claims). `--json` for JSON. It never exits 1 on a
  failing claim; use `check` to gate
- `check --role <role>` prints only what that person has to act on; the exit code does not change:

| role | shows |
|---|---|
| `engineer` | `fail`, with the spec location (file:line) |
| `designer` | `kind: ui` claims that are `unverified` or `fail` |
| `pm` | `stale` and `unapproved` |

## The UI layer

"It looks right" cannot be judged by a machine. Only what can be is carved out:

- **Deterministic**: strings (Figma text nodes = on-screen text), existence of states (variant = story), design token names, transitions, a11y
- **Approximate**: layout (structural diff of screenshots, with a threshold) — not implemented
- **Impossible**: "something feels off" → make it `manual`, surfaced as unverified

The aim is to shrink what designers must check from "every screen" to "only the unverified ones".

## Touch point with non-engineers (PdMs, designers)

They never see YAML. Their only touch point is **approval**:

```
Claims read from docs/spec/rounding.md#端数処理
  ☑ Amounts below one yen are truncated
  ☑ Negative amounts become 0
  ☐ Monthly closing runs on the last day of the month 【to confirm: definition of business day】
```

Show the `extract` output like this; ticking a box = `approve`. A "to confirm" is the discovery that the spec is
not decided yet, which is valuable in itself.

`spec-gate review` is this page: it serves `claims/_pending/` on `http://127.0.0.1:7790` (`--addr`, loopback only;
`--by` names the approver, the OS user by default). Each candidate shows its confidence, the spec excerpt and the
claim in plain language (examples as a table, layers, screen states and strings), never YAML. Approving moves the
file to `claims/spec|arch|ui/` and runs `approve`; when lint or the spec rejects it, the file goes back to `_pending/`
and the reason is shown. Rejecting deletes the candidate. The page uses no CDN or external resources.

### Recording ambiguity

Not every candidate is right or wrong: sometimes the spec itself is not decided. Each candidate on the review page
(and in the remote-approval dialog) has an **ambiguous** button ("曖昧"). It asks for the reason, moves the
candidate to `claims/_ambiguous/` and writes `# ambiguous: <reason>` at the top. Like `_pending/`, `_ambiguous/` is
not checked. `coverage` shows the spec units those claims point to as "to confirm" (`?`); they are a record that the
spec is undecided, so they do not count towards coverage's exit code.

### Review UI (MCP Apps)

Remote sessions cannot open `review` (127.0.0.1), and the question dialog's preview is not shown in the mobile app.
`spec-gate mcp` is an MCP server that shows the pending candidates as an MCP Apps screen (`ui://spec-gate/review`); it starts from the repository's `.mcp.json`.

1. Ask the agent to open the review screen (it calls `mcp__spec-gate__review`)
2. Read each candidate's content, dry-run result, and spec excerpt, then tap approve / reject / ambiguous. "Select bulk-approval targets" → "Approve the N selected" (press twice to confirm) approves several at once

- Candidates are sorted by how easy they are to approve (dry-run pass → higher agreement → example → structure → ui → manual)
- The tools that apply a decision (`review_decide` / `review_bulk_approve`) have `_meta.ui.visibility: ["app"]`: only the screen can call them and the agent does not see them.
  Starting `spec-gate mcp` from a shell to call them directly is denied by hook-gate (`protect_spec_claims`)
- A decision is applied only when the sha256 of the content shown matches the current candidate. Approval runs the same steps as the review page (lint, dry-run, spec hash) and is logged in `.spec-gate/review-decisions.jsonl`
- **Not verified on a device**: whether Claude Code (especially the mobile app) renders MCP Apps is not documented. If it does not, `review` still returns a text summary of the candidates
- The first `go run . mcp` has to build. If the MCP server times out on start, raise `MCP_TIMEOUT` or point `.mcp.json` at a built `spec-gate`

### Remote approval (Claude Code's question dialog)

When you drive Claude Code only through a remote session (for example from a phone), you cannot open that local page.
Remote approval shows each candidate in Claude Code's question dialog (the `AskUserQuestion` tool) with
"承認" (approve) / "却下" (reject) / "曖昧" (ambiguous) buttons and a preview of the candidate, and a PostToolUse hook
hands your answers to spec-gate. The harness produces the answers and starts the hook, so the agent cannot forge
them. No external service, server or network is involved.

**Not yet verified on a real session**: the shape of the answers passed to the hook has not been checked against a
live Claude Code session. Check the result after your first real approval.

1. Add the hook to `.claude/settings.json` yourself (the agent must not edit settings files):

   ```json
   {
     "hooks": {
       "PostToolUse": [
         {
           "matcher": "AskUserQuestion",
           "hooks": [{ "type": "command", "command": "spec-gate review record" }]
         }
       ]
     }
   }
   ```

2. Ask the agent: "Run `spec-gate review ask` and show its output with AskUserQuestion." `review ask [--limit N]`
   prints up to 4 candidates from `claims/_pending/` as the `questions` JSON for the dialog (an empty array, with a
   note on stderr, when there are none).
3. Pick a button for each candidate. To record the reason the spec is undecided, type it into "Other": free text is
   recorded as ambiguous with that text as the reason (see [Recording ambiguity](#recording-ambiguity)).

`review record` applies an answer only when the question shown is exactly the one spec-gate builds from the
candidate now (including the preview), so a question whose content the agent altered is ignored. Answers are read
only from `tool_response`; `tool_input.answers` is never used, because the agent fills `tool_input` itself and could
forge an approval there (when the answers are only in `tool_input`, nothing is applied). Approve, reject and
ambiguous go through the same steps as the review page. Applied decisions are appended to
`.spec-gate/review-decisions.jsonl` (time, candidate id, decision, reason, approver, sha256 of the preview shown), and
the result is passed back to Claude as `additionalContext`. The hook always exits 0 so it never breaks the dialog.
hook-gate denies the agent running `review record` itself and calling the review page's `/api/approve`,
`/api/reject` and `/api/ambiguous` from the shell.

## Design limits and what is not implemented

- **Whether a claim is right is finally a human decision.** Consensus, lint and coverage reduce mistakes before
  a human sees them; they are not a guarantee. When a bug reaches production, trace it back to the claim
  (wrong claim / missing claim / weak check)
- Detecting `structure` violations relies on a full depgraph scan. Debt outside the claim's layers (cycles,
  duplication, ...) is watched by `depgraph gate` in the quality gate (only what the session added) and depgraph's UI
- `ui` string verification assumes Storybook or the app is running and needs `--ui`. In CI, use a separate job
- `extract` and `reverse` depend on the model that answers the prompts (a subagent, or the command in
  `SPEC_GATE_EXTRACT_CMD` / `SPEC_GATE_REVERSE_CMD`). `reverse` is a review aid for humans: by default it always
  exits 0
- Holdout hides expected values, not inputs: `in` is plain text so it can be fed to the binding. The read protection
  of `holdout/` relies on Claude Code permissions and hook-gate rules
- `fuzz` generates numbers only (`number` / `integer` in a range)
- Miro depends on exported images and is the weakest. Prefer Mermaid exports in practice

## Division of roles with hook-gate / depgraph

- hook-gate: how to walk. The runtime for Stop / PostToolUse / PreToolUse, protection of claims, verify lines and holdout
- depgraph: the dependency detector. Cycles, layer violations, duplication
- spec-gate: whether "what to build" is honoured. example / ui / plan are the core; structure only checks the diagram against the depgraph settings

## Development

The toolchain (Go, golangci-lint, node, pnpm, goreleaser, actionlint) is pinned in `mise.toml`.

```bash
mise run setup            # first time
mise run ci               # what CI runs: vet, lint, lint:actions, test, build, licenses:check, release:check, spec:check
mise run release:snapshot # build the release archives into dist/ without publishing
```

Pull request titles must be [Conventional Commits](https://www.conventionalcommits.org/) (`feat: ...`, `fix: ...`):
the squash-merge commit takes the PR title, and release-please builds the changelog and the next version from it.

## License

Apache-2.0 (see [LICENSE](LICENSE) and [NOTICE](NOTICE)). The licenses of the Go modules linked into the binary are in
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
