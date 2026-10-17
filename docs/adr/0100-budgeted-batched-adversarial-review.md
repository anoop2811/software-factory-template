# ADR 0100: budgeted batched adversarial review

Status: accepted direction; implementation and qualification pending
Date: 2026-10-17

## Context

The user authorized the six-part reliability proposal and required the change
itself to receive model review. PR #126's 395692-byte diff exceeded the 200000-byte
guard before any provider call; PR #124 received a completed model review.
Evidence: https://github.com/anoop2811/software-factory-template/pull/126#issuecomment-6236988356
and https://github.com/anoop2811/software-factory-template/pull/124#issuecomment-6031211962.
PR #126 remains separate. This repair branches from the current trusted main.

## Decision and public contract

Extend the existing Go streaming client with deterministic bounded diff batches,
exact-commit coverage, authenticated successful cache reuse, optional approved
same-model provider fallback, run-wide budgets and explicit completion status.
Use independent outside-in Ginkgo/Gomega RED before production code. Keep the
same shared command for Codex, Claude Code and OpenCode; no new module dependency.

Add Cobra `factory review-batch --diff FILE --files FILE --base SHA --head SHA
--repository OWNER/REPO --output DIR [--cache DIR] [--plan]`. Inputs are literal
data; output is a newly created private directory with report.json and review.md.
Plan reports planned work without calling a model. Root configuration loading
reuses config.ExportPlan, preserving existing environment/model precedence.

Parse complete Git unified diffs and validate hunk counts and a complete flattened
GitHub files manifest (filename, previous_filename, status, additions, deletions).
Reject missing/duplicate/truncated/malformed data before calls. Optional API patch
fields are not the diff oracle. Group related paths deterministically; split
oversized files at hunk/line boundaries with fragment identity and coordinates.
Never silently truncate. Nontext/unreviewable changes remain explicit uncovered
items; no such item can establish complete coverage. Admit at most 8 MiB input
and 3000 manifest files. Paths are data and never filesystem output names.

Shared context contains a deterministic manifest and hunk index. Cache identity
includes repository, base, batch content, shared context, model, prompt and all
limits/routing. A new head can reuse unchanged matching work with a new report;
changed shared context invalidates affected results. Cache entries authenticate
their full identity, findings and nullable usage with HMAC derived from the review
key. Only completed results are cached. Invalid/linked/oversized/mismatched entries
cannot manufacture coverage. Cache directories and output files remain confined
private regular artifacts; no secret or reasoning text is stored.

## Limits and routing

Serial execution is the default. One total deadline includes backoff and all
batches. Preserve existing per-call REVIEW_MAX_TOKENS (configured 32768), model,
reasoning and explicit provider selection. Every actual network attempt reserves
its requested response cap and request bytes before sending. Reservations are
conservative allowances, not fabricated usage or a guarantee of provider billing.
Track reported usage separately; unknown remains null.

| Environment / Actions variable | Default | Range |
|---|---|---|
| REVIEW_BATCH_BYTES | 60000 | 4096..100000 patch bytes |
| REVIEW_MAX_BATCHES | 16 | 1..64 |
| REVIEW_MAX_REQUESTS | 16 | 1..64 attempts including retries |
| REVIEW_TOTAL_MAX_TOKENS | 524288 | 1024..2097152 reserved output tokens |
| REVIEW_TOTAL_INPUT_BYTES | 2097152 | 4096..16777216 reserved request bytes |
| REVIEW_TIMEOUT_SECONDS | 1200 | 1..1200 seconds for entire run |
| REVIEW_FALLBACK_PROVIDERS | empty | Up to three additional explicit slugs |
| REVIEW_MAX_PROMPT_PRICE | empty | Positive decimal USD per million input tokens |
| REVIEW_MAX_COMPLETION_PRICE | empty | Positive decimal USD per million output tokens |

Fallback is off by default and requires both price ceilings, an explicit provider
allowlist, same-model routing and require_parameters=true. Existing strict single
requests stay compatible. Keep the fixed endpoint and existing initial 429/503
retry, honor Retry-After, and never automatically replay ambiguous generation.
Only a fully terminated nonempty stream counts; no partial findings on failure.

## Coverage, CI and qualification

Versioned reports record exact repository/base/head, policy/context identity,
planned/completed/cached counts, ordered batch and file/fragment coverage, safe
failure reasons, budget reservations and nullable reported usage. Markdown is a
deterministic projection. Complete means all eligible fragments completed and no
unreviewable item remains; partial/failed/skipped are explicit. Exit 0 means a
complete review or a requested offline plan; incomplete reviews are nonzero.

Keep pull_request_target executable code on the trusted base. Check revision
identity around API data reads and before posting. Never execute/source/build
PR-head code with secrets. Dedicated cache artifacts authenticate results; expose
a separate non-required completion check and retain coverage/findings artifacts.
Provide an explicit rerun path. The model's opinion remains advisory.

Qualification includes malformed/truncated diff, large-file fragmentation, budget
exhaustion, 429/503, interrupted streams, cancellation, tampered cache and changed
context. Run review-diamond correctness/security/tests, verification, deterministic
dedupe and synthesis. Then observe a bounded real model review of the exact final
change, fix actionable findings and obtain review again after corrections.

Authoritative APIs fetched 2026-10-17 before implementation:
https://openrouter.ai/docs/guides/routing/provider-selection
https://openrouter.ai/docs/api_reference/errors-and-debugging
https://docs.github.com/en/actions/reference/security/secure-use
https://docs.github.com/en/rest/checks/runs?apiVersion=2022-11-28

## Shared hosted adapter

`factory review-batch github --pr NUMBER --repository OWNER/REPO --expected-base
SHA [--expected-head SHA] --output DIR [--cache DIR]` owns GitHub data retrieval,
revision checks, calling the same batch engine, advisory posting and a separate
check run in Go. It uses GITHUB_TOKEN only at fixed api.github.com endpoints and
REVIEW_API_KEY only at OpenRouter. It validates same-repository PRs targeting the
repository default branch; base must match trusted checkout. Optional head is
bound from the initial metadata for manual dispatch. Metadata is checked again
after diff/manifest reads and immediately before publication. Refuse stale work.
The complete file count must match changed_files, with bounded pagination.
No PR path or payload is executed. Unsafe HTTP redirects and raw error logging
remain disabled. The completion check succeeds only for complete current-head
coverage; incomplete/error outcomes fail the optional check, explicit skips skip
it. Findings are advisory. Long comments are labeled excerpts linking complete
run artifacts. Missing credentials and preparation failure never imply review.
Use this shared adapter in both repository and generated Go-enabled workflows;
retain explicitly selected legacy adopter behavior until its normal Go upgrade.
