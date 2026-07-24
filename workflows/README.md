# workflows/

Reusable **workflow recipes** — the harness-agnostic substrate of "graph
engineering." A recipe is a plain-text graph: nodes (roles) connected by edges
(data), with a verifier before findings count. Your agent reads a recipe and runs
it with whatever orchestration its harness has (Claude Code workflows, opencode
subagents, Codex `spawn_agent`) — the *same* recipe on any of the three.

`scripts/hooks/workflow-lint.sh` checks every recipe for graph hygiene: real
roles, plumbing expressed as edges (not agents), a verifier present, fan-outs that
declare what they run over. It runs in `make check` and CI, and fires only if this
directory has recipes — opt-in by construction.

## Format

Each `## <node>` block declares:

- `- role:` — a factory role (`spec-writer`, `implementer`, `reviewer`,
  `refactorer`, `wiki-maintainer`) or `code` for deterministic plumbing.
- `- kind:` — `agent` (one node), `fanout` (parallel, needs `- over:`),
  `verify` (a skeptic before findings count), or `edge` (plumbing — must be
  `role: code`).

See the shipped `review-diamond.md` and `eval-fanout.md`, and
[docs/WORKFLOWS.md](../docs/WORKFLOWS.md) for how each harness runs a recipe.
