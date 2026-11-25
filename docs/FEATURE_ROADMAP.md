# Developer capability roadmap

Provenance: user-selected roadmap and priority order in the 2026-09-06 factory
development session. Keep these feature names stable when reporting progress.
v0.2.0 introduced the Go CLI, authenticated setup and upgrades, harness configuration,
project and CI checks, budgets, repair loops, metrics and optional model review.
The table tracks additional capabilities against the original roadmap. A 0% row
can have useful foundations already. For example, the Go factory includes Markdown
workflow recipes and golden-task evaluation. The executable graph engine and
automated model-selection proposals below are future work.
Completion means the defined implementation and acceptance checks are complete;
release and live-runtime evidence are reported separately. They are not effort
estimates. Update only from observed acceptance results.

| Priority | Concept | Proposed addition | User-selectable options | Cost-conscious default | Completion |
|---|---|---|---|---|---|
| P0 — First | Budgeting and observability | Run plan, enforced invocation limits, truthful usage metadata and reports | Task/session limits; estimate warning or stop; JSON export | No model calls for reporting; unknown usage stays unknown | 100% — shipped in v0.2.0, deterministic acceptance (Decision 46) |
| P1 | Loop engineering | Bounded implement/test/review/repair, progress detection and checkpoints | Manual, bounded repair, explicitly unattended | Manual first; limited opt-in attempts | 100% — shipped in v0.2.0, deterministic acceptance (Decision 47) |
| P1 | Context engineering and Skills | [Task-specific context and on-demand procedures](../specs/003-task-context-and-skills.md) | Explicit files, local retrieval, optional summaries | Local selection and bounded context | 0% |
| P1 | Memory engineering | Scoped records, provenance, freshness, supersession and handoffs using [Spec 003 for retrieval](../specs/003-task-context-and-skills.md#memory-handoff) | Off, session, project, explicitly shared team | Local files and lexical search | 0% |
| P1 | Harness engineering | Capability probes, isolation and evidence tied to tested code | Existing gates, isolated execution, stricter boundaries | Shared scripts and native controls | 0% |
| P2 — Next | Graph engineering | Executable resumable recipes, typed handoffs and deterministic aggregation. [First proposed pilot](#work-order-pilot) | Serial, bounded parallel, task-specific graphs | One active role; preserve role separation | 0% |
| P2 | RAG / repository retrieval | Cited fresh code, specs, ADRs and approved external documents | Keyword/symbol, hybrid embeddings, external sources | Local search; incremental optional indexing | 0% |
| P2 | MCP and tool engineering | Integration catalog, permissions, capabilities and bounded outputs | Off, selected read-only, permitted writes | Selected tools and supported lazy discovery | 0% |
| P2 | Eval-driven model and harness selection | Compare quality, retries, latency and reported cost on realistic tasks | Manual, configuration-triggered, scheduled | Small manual suites; reviewed recommendations | 0% |
| P3 — Opt-in | Continuous agentic maintenance | Issue reproduction, docs/dependencies and release preparation | Manual, weekly, selected events | Off; bounded runs and duplicate suppression | 0% |
| P3 — Specialist | Knowledge graphs / GraphRAG | Cross-service and cross-document relationship retrieval | Off, deterministic dependency graph, GraphRAG | Off until measured retrieval value justifies cost | 0% |

All additions target Codex, Claude Code and OpenCode through shared logic and
thin adapters. Unsupported native capabilities must be disclosed, never silently
dropped. Feature selection is separate from the existing standard/economy model
profiles; no quality gate is relaxed by selecting a cheaper profile.

For P0, the five deliverables and acceptance boundary are in [BUDGETS.md](BUDGETS.md).

For P1 Loop engineering, bounded mode is explicit unattended execution with finite
limits; terminal runs hand off rather than automatically restart. Fresh manual
checks may resume. See [LOOPS.md](LOOPS.md) for completion scope and evidence.

### Work-order pilot

Proposed first use case for graph engineering. A developer runs one command with an approved
local work order to prepare a PR for human review. Use [Spec 003's role-specific context](../specs/003-task-context-and-skills.md#work-order-evidence)
and the [bounded loop](LOOPS.md#acceptance-contract). The plan shows the outcome, roles, budget
and approval points with zero model calls. If the order declares several independent outcomes,
return it for a scope decision before launching an agent. One observable behavior fits one PR.

A local ticket export can supply that work order. For example, a Jira Story can carry its linked
Aha! product requirement, and a Linear Issue can carry its Project scope and Initiative objective. Follow the team's
[work-item mapping](../specs/003-task-context-and-skills.md#work-item-mappings). The assigned ticket
sets the scope. A reviewer can trace its criteria to the exported record and revision.

For a behavior change, have the spec-writer establish an independent test and record its expected
failure before handing it to the implementer. Reuse the loop for implementation, checks, review
and bounded repair. For a docs-only change, use the relevant document checks. Select extra roles
only when the work calls for them. Apply the same contract to Codex, Claude Code and OpenCode.

Prepare the PR description locally by default. Tie each acceptance criterion to the applicable
test evidence, the checked source snapshot and the review verdict. A stale snapshot or exhausted
budget yields a handoff with the reason. Publishing a draft PR requires explicit opt-in.
Report remote CI for the published revision separately, and preserve required human approvals.

Pilot one real factory issue against the current manual handoff process. Record manual handoffs,
model calls and whether a reviewer could decide from the evidence without requesting missing proof.
