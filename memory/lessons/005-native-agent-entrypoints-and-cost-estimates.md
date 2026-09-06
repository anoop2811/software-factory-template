# Native entrypoints and cost estimates need separate checks

The same role name does not guarantee the same runtime configuration. OpenCode's
run command rejects a subagent-only role and selects its default agent; invoking
Codex at the root does not select a generated subagent TOML just because the
prompt names that role. Budget adapters must establish what configuration the
actual entrypoint loads. Decision 46 and docs/BUDGETS.md own the contract.

Similarly, Claude's total_cost_usd is a client-side estimate. A structured field
with a dollar name is not authoritative billing, and a post-run usage report
does not establish a strict pre-run financial ceiling.

Provenance:

- OpenCode run source fetched 2026-09-06:
  https://raw.githubusercontent.com/anomalyco/opencode/dev/packages/opencode/src/cli/cmd/run.ts
- Codex root CLI help observed 2026-09-06 via `codex exec --help`; generated
  reviewer sandbox and implementer hooks are in .codex/agents/reviewer.toml and
  .codex/agents/implementer.toml, while .codex/config.toml registers subagents.
- Claude cost semantics fetched 2026-09-06:
  https://code.claude.com/docs/en/agent-sdk/cost-tracking
