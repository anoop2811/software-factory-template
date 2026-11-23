# Workflows

A workflow recipe describes how a team wants coding agents to carry out a
task. The factory ships recipes for review and evaluation and a linter for their
structure. The active harness coordinates the work with its available tools.
For the factory's executable check and repair loop, see [LOOPS.md](LOOPS.md).

## A recipe is a plain-text graph

`workflows/<name>.md`. Each `## <node>` block declares:

- `- role:` — a factory role (`spec-writer`, `implementer`, `reviewer`,
  `refactorer`, `wiki-maintainer`), or `code` for deterministic plumbing.
- `- kind:` — `agent` (one node) · `fanout` (parallel; needs `- over:`) ·
  `verify` (a skeptic before findings count) · `edge` (plumbing; must be
  `role: code`).

The shipped `review-diamond.md` (fan out reviewers by lens → verify each finding
→ merge in code → synthesize) and `eval-fanout.md` are the worked examples.

## The gate: `workflow-lint`

`scripts/hooks/workflow-lint.sh` runs in `make check` and CI, and enforces the
graph hygiene from the playbook — deterministically, and identically on every
harness, because it checks the shared recipe:

- every node names a real role or `code`;
- plumbing (dedupe/merge/flatten/combine/filter/aggregate) is an **edge**, not an
  agent — a graph where every edge is an agent pays rent on its own wiring;
- an `edge` is `role: code` — coordination is code, not a conversation;
- a `fanout` declares what it runs `over:`;
- every recipe has a `verify` node — findings are checked before they reach output.

It fires only when `workflows/` has recipes, so it is opt-in by construction.

## How each harness runs a recipe

The recipe is a **definition, not an engine** — so each harness runs the same
graph its own way. `AGENTS.md` points every harness's agent at `workflows/`:

- **Claude Code** — a dynamic workflow: a JS orchestration script whose
  coordination costs zero model tokens. Generating one from a recipe into
  `.claude/workflows/` is an optional, Claude-only optimization — not required
  for the recipe to work.
- **opencode** — subagent dispatch: the orchestrator spawns one subagent per
  fan-out node.
- **Codex** — `spawn_agent` / `wait_agent`: the same fan-out via Codex's
  multi-agent tools.

All three harnesses can read the same recipe. Actual dispatch depends on the
harness, permissions and tools available in the session. See the
[roadmap](FEATURE_ROADMAP.md) for proposed resumable graph execution.
