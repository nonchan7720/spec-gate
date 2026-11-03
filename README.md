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
| Extract | `spec-gate extract <ref> --runs 3` | AI | Reads the same spec N times independently; agreed claims go to `claims/_pending/` with high confidence (consensus) |
| Lint | `spec-gate lint` | machine | Do examples satisfy properties; do claims contradict each other |
| Approve | `spec-gate approve <id>` | **human** | Shows the spec excerpt and freezes its hash. Refused when lint fails |
| Review | `spec-gate review` | **human** | A local web page (127.0.0.1) listing `claims/_pending/` in plain language with the spec excerpt; ticking and approving moves each claim into `claims/<area>/` and runs `approve` |
| Check | `spec-gate check` | machine | approval → spec drift → per-kind check → plan verify |
| Coverage | `spec-gate coverage` | machine | Lists the parts of the specs no claim covers |
| Reverse | `spec-gate reverse [<id>...]` | AI | Rewrites each claim as spec prose without seeing the spec, then compares it with the original excerpt and lists what the claim adds, misses or contradicts (review aid; `--strict` exits 1 on a mismatch) |

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

### A structure that cannot lie

- **The AI cannot touch claims.** Editing `claims/**` and `.spec-gate/`, and running `approve` / `plan freeze`,
  are denied by hook-gate `pre_edit` / `pre_bash` rules (`.claude/feedback/protect_spec_claims.md`)
- **Plan verify lines are frozen first.** `spec-gate plan freeze` records the hash of the verify lines; loosening
  them after implementation makes the plan `stale`. On top of that, right after a plan is edited, a hook-gate
  `post_edit` rule runs `spec-gate plan check` and stops the agent on the spot when a frozen verify line changed
  (`.claude/feedback/protect_plan_verify.md`)
- **A changed spec becomes stale automatically**, so the spec cannot be rewritten to make things fit
- **A claim without a means of verification is unverified, not fail.** Nothing "passes without being checked";
  instead it is surfaced as something a human should look at

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
./spec-gate approve ROUND-001 ROUND-002 CLOSE-001 ARCH-001 UI-001
./spec-gate check                          # full verdict
./spec-gate coverage                       # what is missing
./spec-gate plan check plans/x.md          # has a frozen plan's verify changed
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
- At stop, the `consistency_checks` `go-all` (build / golangci-lint / all tests) and `spec-gate`
  (`check --changed --hook`) run and block the stop on failure. Giving up after repeated failures (5 times)
  is hook-gate's job

depgraph uses the Neo4j started by `docker compose up -d` (`localhost:17687`); the connection is set in `[env]` of
`mise.toml`, and depgraph is always called through `mise exec --` (hooks and gate). The SessionStart hook in `.claude/settings.json`
(`depgraph hook session-start`) records the baseline at session start, and `depgraph gate` fails only on debt added since then.

`CLAUDE.md` holds the rules for the agent.

## Layout

```
docs/spec/*.md        detailed specs (Markdown)        ─┐
docs/arch/*.mmd       architecture diagrams (Mermaid)   ├ sources
Figma / *.png / *.pdf  UI and others                   ─┘
sources.yaml          registry of watched sources (for coverage)
claims/<area>/*.yaml  claims. Format: claims/SCHEMA.md
claims/_pending/      extract output. Not checked until approved
plans/*.md            plans. Checked when the front matter says status: active
.spec-gate/           plan locks (committed)
.depgraph.yml         depgraph's layer definition. The structure claim checks it matches the diagram
.claude/gate.yaml     hook-gate quality gate
.claude/feedback/     hook-gate protection rules (claims, verify lines)
tools/call.ts         binding wrapper for example claims (TS functions)

main.go               entry point
pkg/domain            Claim / Plan / Result, ports, lint (pure)
pkg/services          check / approve / coverage / extract / plan
pkg/infrastructure    claimrepo, adapter(markdown/mermaid/figma/image), checker, planfile, vcs, extractor
pkg/interfaces/cli    cobra commands, reports, hook output
```

## Claim kinds and what can be enforced mechanically

| kind | example source | verification | confidence |
|---|---|---|---|
| `example` | detailed specs (rounding, closing dates) | Feeds input/output examples to the binding command and evaluates properties (expr) | high, deterministic |
| `structure` | Mermaid diagram | Checks that the diagram's edges (= allowed) agree with `layers` / `forbidden` in `.depgraph.yml`; detecting violations is left to `depgraph gate` | the comparison is deterministic; detection depends on depgraph |
| `ui` | Figma | A story export with the same name as each variant / with `--ui`, Playwright checks the strings | medium |
| `manual` | anything | none; always unverified | — |

The `example` binding is any command honouring "stdin JSON → stdout JSON", so the same claim verifies TS, Go or Python.

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

## Design limits and what is not implemented

- **Whether a claim is right is finally a human decision.** Consensus, lint and coverage reduce mistakes before
  a human sees them; they are not a guarantee. When a bug reaches production, trace it back to the claim
  (wrong claim / missing claim / weak check)
- Detecting `structure` violations relies on a full depgraph scan. Debt outside the claim's layers (cycles,
  duplication, ...) is watched by `depgraph gate` in the quality gate (only what the session added) and depgraph's UI
- `ui` string verification assumes Storybook or the app is running and needs `--ui`. In CI, use a separate job
- `extract` assumes `claude -p`. Override with `SPEC_GATE_EXTRACT_CMD`
- `reverse` assumes `claude -p`. Override with `SPEC_GATE_REVERSE_CMD`. It is a review aid for humans: by default it
  always exits 0, and its quality depends on the model
- Miro depends on exported images and is the weakest. Prefer Mermaid exports in practice

## Division of roles with hook-gate / depgraph

- hook-gate: how to walk. The runtime for Stop / PostToolUse / PreToolUse, protection of claims and verify lines
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
