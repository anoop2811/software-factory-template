# Developer capability roadmap

Provenance: user-selected roadmap and priority order in the 2026-09-06 factory
development session. Keep these feature names stable when reporting progress.
The percentages measure the proposed additions, not the pre-existing scaffold.
Completion means the defined implementation and acceptance checks are complete;
release and live-runtime evidence are reported separately. They are not effort
estimates. Update only from observed acceptance results.

| Priority | Concept | Proposed addition | User-selectable options | Cost-conscious default | Completion |
|---|---|---|---|---|---|
| P0 — First | Budgeting and observability | Run plan, enforced invocation limits, truthful usage metadata and reports | Task/session limits; estimate warning or stop; JSON export | No model calls for reporting; unknown usage stays unknown | 100% — implementation and deterministic acceptance; Decision 46 |
| P1 | Loop engineering | Bounded implement/test/review/repair, progress detection and checkpoints | Manual, bounded repair, explicitly unattended | Manual first; limited opt-in attempts | 0% |
| P1 | Context engineering and Skills | Task-specific context and on-demand procedures | Explicit files, local retrieval, optional summaries | Local selection and bounded context | 0% |
| P1 | Memory engineering | Searchable scoped memory, provenance, freshness and handoffs | Off, session, project, explicitly shared team | Local files and lexical search | 0% |
| P1 | Harness engineering | Capability probes, isolation and evidence tied to tested code | Existing gates, isolated execution, stricter boundaries | Shared scripts and native controls | 0% |
| P2 — Next | Graph engineering | Executable resumable recipes, typed handoffs and deterministic aggregation | Serial, bounded parallel, task-specific graphs | One active role; preserve role separation | 0% |
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
