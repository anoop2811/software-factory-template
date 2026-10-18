# ADR 0100: budgeted batched adversarial review

Status: accepted
Date: 2026-10-17

## Context

User approval requires real model review of this change.
PR #126: 395692 bytes exceeded the 200000-byte guard.
No provider call ran. PR #124 received a completed review.
Evidence: https://github.com/anoop2811/software-factory-template/pull/126#issuecomment-6236988356
and https://github.com/anoop2811/software-factory-template/pull/124#issuecomment-6031211962.
PR #126 stays separate; this repair branches from trusted main.

## Decision and public contract

Extend the Go streaming client with deterministic bounded batches,
exact-commit coverage, authenticated cache reuse, optional approved
same-model fallback, run-wide budgets and explicit completion.
Require independent outside-in Ginkgo/Gomega RED before code. Keep one
command for Codex, Claude Code and OpenCode; no new dependency.

Add Cobra `factory review-batch --diff FILE --files FILE --base SHA --head SHA
--repository OWNER/REPO --output DIR [--cache DIR] [--plan]`. Inputs are literal
data; create private output with report.json and review.md.
Plan mode makes no model calls. Root config loading
reuses config.ExportPlan and its environment/model precedence.

Validate complete Git unified diffs, hunk counts and flattened
GitHub files manifest (filename, previous_filename, status, additions, deletions).
Reject missing/duplicate/truncated/malformed input before calls. API patch
fields are not the oracle. Group related paths deterministically; split
large files at hunk/line boundaries with fragment IDs and coordinates.
Never truncate silently. Nontext/unreviewable changes stay uncovered;
they prevent complete coverage. Admit at most 8 MiB input
and 3000 files. Paths never become output filenames.

Shared context is the deterministic manifest and hunk index. Identity
includes repo, base, batch content, context, model, prompt and all
limits/routing. Matching work can be reused under a new head/report;
changed context invalidates results. Cache HMAC authenticates
identity, findings and nullable usage using the review
key. Cache only completed results. Invalid/linked/oversized/mismatched entries
cannot create coverage. Keep cache directories and output files confined
and private; never store secrets or reasoning.

## Limits and routing

Run serially. One deadline includes backoff and all
batches. Preserve REVIEW_MAX_TOKENS (configured 32768), model,
reasoning and provider selection. Before each network attempt reserve
the response cap and request bytes before sending.
Top-level usage counts current-run calls; cached batch usage is historical.
Any unknown attempted-call usage makes the total null; known batch values remain.

| Variable | Default | Range |
|---|---|---|
| REVIEW_BATCH_BYTES | 60000 | 4096..100000 patch bytes |
| REVIEW_MAX_BATCHES | 16 | 1..64 |
| REVIEW_MAX_REQUESTS | 16 | 1..64 including retries |
| REVIEW_TOTAL_MAX_TOKENS | 524288 | 1024..2097152 output tokens |
| REVIEW_TOTAL_INPUT_BYTES | 2097152 | 4096..16777216 request bytes |
| REVIEW_TIMEOUT_SECONDS | 1200 | 1..1200 seconds/run |
| REVIEW_FALLBACK_PROVIDERS | empty | Up to three extra slugs |
| REVIEW_MAX_PROMPT_PRICE | empty | Positive USD/million input tokens |
| REVIEW_MAX_COMPLETION_PRICE | empty | Positive USD/million output tokens |

Fallback defaults off. Require both price ceilings, an explicit provider
allowlist, same-model routing and require_parameters=true. Strict single
requests stay compatible. Keep the fixed endpoint and initial 429/503
retry; honor Retry-After; never replay ambiguous generation.
Only terminated nonempty streams count; discard partial failed findings.

## Coverage, CI and qualification

Versioned reports identify repository/base/head, policy/context,
planned/completed/cached counts, ordered batch/file/fragment coverage, safe
failure reasons, reservations and nullable usage. Markdown is a
deterministic projection. Complete requires every fragment and no
unreviewable items; label partial/failed/skipped. Exit 0 means a
complete review or requested offline plan; otherwise exit nonzero.

Run pull_request_target code from trusted base. Check revision
identity around reads and before posting. Never execute/source/build
PR-head code with secrets. Authenticate cached results; expose
an optional completion check and retain coverage/findings artifacts.
Provide explicit reruns. Model opinions remain advisory.

Test malformed/truncated diffs, fragmentation, budget
exhaustion, 429/503, interrupted streams, cancellation, tampering and changed
context. Run review-diamond correctness/security/tests, verification,
deterministic dedupe and synthesis. Observe real model review of the final
change; fix actionable findings and re-review corrections.

Authoritative APIs fetched 2026-10-17:
https://openrouter.ai/docs/guides/routing/provider-selection
https://openrouter.ai/docs/api_reference/errors-and-debugging
https://docs.github.com/en/actions/reference/security/secure-use
https://docs.github.com/en/rest/checks/runs?apiVersion=2022-11-28

## Shared hosted adapter

`factory review-batch github --pr NUMBER --repository OWNER/REPO
[--expected-base SHA] [--expected-head SHA] --output DIR [--cache DIR]` retrieves data,
checks revisions, calls the batch engine, posts findings and a separate
check in Go. GITHUB_TOKEN goes only to fixed api.github.com endpoints;
REVIEW_API_KEY only to OpenRouter. Accept same-repo PRs into the
default branch. Bind omitted base/head from initial metadata for dispatch;
executable checkout stays trusted event-base/default-branch code. Recheck metadata
after diff/manifest reads and before publication. Reject stale work.
Match file count to changed_files; bound pagination.
Never execute PR data. Keep unsafe redirects and raw error logging
disabled. The optional check succeeds only for complete current-head
coverage; incomplete/error outcomes fail, explicit skips skip
it. Findings stay advisory. Label long comments as excerpts linking full
artifacts. Missing credentials/preparation failure never imply review.
Share this adapter across repository and generated Go-enabled workflows;
retain explicit legacy behavior until the adopter's normal Go upgrade.
