# ADR 0112: complete bounded reviews within the hosted lane deadline

Status: implemented with local qualification; hosted performance measurement pending

Date: 2026-10-29 UTC

## Context and evidence

The operator requires complete adversarial review within twenty minutes, not a
sequence of twenty-minute partial runs. PR #141's exact-head reports observed
14 batches and 129 files. Sequential model calls included 599-second and
697-second attempts. A later run reused six batches and completed six more
before the same 1,200-second deadline. Its saved cache proves checkpointing;
it does not qualify uncached end-to-end latency.

OpenRouter's model metadata fetched 2026-10-29 identifies GLM 5.3 Flash reasoning
as mandatory, with supported efforts `max`, `high` and `low`. Keep the selected
model, `low` effort and pinned DeepInfra routing. Do not invent support for
`none`, `minimal` or a strict reasoning-token budget for this model.

The final cached-resume run completed all fourteen batches; its two remaining
requests took 221.158 and 459.519 seconds. Use an explicit 600-second repository
batch allowance with four workers, retaining the existing total-only CLI bound.
That accommodates an observed legitimate response without extending the global
hosted deadline. It does not prove uncached performance before measurement.

Sources fetched 2026-10-29:

- https://openrouter.ai/api/v1/models
- https://openrouter.ai/api/v1/models/z-ai/glm-5.3-flash/endpoints
- https://openrouter.ai/docs/guides/best-practices/reasoning-tokens
- https://docs.github.com/en/actions/reference/workflows-and-actions/dependency-caching

GitHub gives default-branch `pull_request_target` jobs read-only cache access
unless explicitly configured otherwise. The current job's save step can report
success while its log warns that saving was denied. A trusted manual run saved
the same completed, authenticated batches. Correct the automatic lane without
executing PR-head code or exposing secrets to a preparation step.

## Execution contract

Add `REVIEW_PARALLELISM`, default 1, accepting integers 1 through 4. Keep direct
CLI behavior serial unless explicitly selected. The repository selects four
workers through its Actions variable; generated workflows expose the same
option and retain the conservative default. Bound actual in-flight requests,
not merely goroutine count. A single-worker run preserves early-stop behavior.

Add `REVIEW_BATCH_TIMEOUT_SECONDS`, default 1200, accepting 1 through 1,200
seconds. Each batch, including its permitted HTTP backoff/retry, gets the
smaller of that allowance and the remaining run deadline. Cancellation must
interrupt its real HTTP request. Do not retry an ambiguous generation, accept
truncated output, discard completed sibling results, or label a timeout clean.

Keep all existing request-count, aggregate-response-token, aggregate-input,
per-response, model and provider limits. Every actual network attempt must
atomically reserve the existing shared budgets before sending. Parallel
workers cannot each spend a separate copy of the run allowance. Use standard
Go synchronization and the existing streaming client; add no dependency.

Cache results are still authenticated against complete policy/context identity.
Keep report and finding order deterministic regardless of completion order.
Only complete results become cached coverage. An attempted call with unknown
usage keeps current-run total usage unknown. Preserve redaction, private
directories, bounded response capture and exact-head publication checks.

Stop admitting new paid work after a failed batch or exhausted budget; collect
already admitted work within its existing deadline and retain its complete
results. Serialize shared diagnostic writes and report mutation. There must be
no race on budget accounting, cache state, usage or result publication.

Emit serialized progress acknowledgements identifying the batch number/count,
completed status and elapsed seconds. These fixed metadata records let operators
identify a slow batch and let independent fixtures observe failure processing
before releasing a held sibling. Never include findings or raw provider errors
in this progress channel.

## Hosted execution

Keep executable checkout and client build on the trusted base/default branch.
The dedicated review job may explicitly request cache write access because it
executes only that trusted client, treats all PR content as data, writes only
the dedicated completed-batch cache and authenticates every restored result.
Keep unrelated build/tool caches outside this job. Never cache credentials or
reasoning. Independently review this narrow trust decision.

The latest tagged actionlint release fetched from its official releases API on
2026-10-29 is v1.7.12 (2026-03-30), whose parser does not recognize GitHub's newer
job-level `cache-mode`. Keep the qualified release and suppress only that exact
unsupported job-key diagnostic; independent workflow criteria must validate the
actual key's scope/value and trusted execution. Do not ignore unrelated schema,
expression or shell errors. Source:
https://github.com/rhysd/actionlint/releases/tag/v1.7.12

Expose both new settings in repository and generated native-review workflows.
Use an 18-minute default review execution allowance and a 20-minute job ceiling
to leave setup/publication headroom. Preserve configurable stricter allowances;
neither workflow may silently extend the operator's twenty-minute job ceiling.
No timeout or partial result may become a successful completion check.

The privileged workflow must receive this repair on trusted main before it can
review the larger cutover with the new client. Use a small prerequisite PR with
its own independent review and tests; do not substitute PR-head execution with
secrets. Keep the cutover as the active objective and carry no new feature work
past it.

## Qualification

An independent Ginkgo/Gomega author must observe failures before implementation
for bounded request overlap, invalid settings and per-batch cancellation.
Exercise concurrent shared-budget exhaustion, HTTP retry reservations,
out-of-order completion, sibling failure, authenticated resume, cancellation,
redaction and deterministic coverage. Use real controlled HTTPS handshakes and
bounded release channels rather than a timing-only speed assertion. Existing
review, workflow, routing and hosted exact-revision criteria remain required.

Observe a real complete current-head model review with elapsed time below
twenty minutes, recording cached and uncached counts. An all-cached replay
proves resumption only. Provider outages can still fail within the deadline;
they never justify fabricating complete coverage. Implementation, local fixture
passes and an intended worker count are not evidence of hosted performance.

## Local qualification

The independent held-HTTPS overlap criterion failed before implementation:
`0 Passed | 1 Failed` in 3.213 seconds. The two workflow criteria likewise failed
before their configuration changed. The final selection of twenty parallel
criteria, two new workflow criteria and seven existing workflow criteria passed
`29 Passed | 0 Failed` in 25.168 seconds with evaluator and CLI race instrumentation.
The same selection on the isolated trusted-main repair passed all 29 in 27.624
seconds, independently of the Go-cutover changes.

The broader main-based review regression command was:

```sh
FACTORY_CLI_TEST_RACE=1 GOMAXPROCS=2 GORACE=atexit_sleep_ms=0 \
  go test -race -count=1 -timeout=5m -v ./acceptance -ginkgo.no-color \
  -ginkgo.focus='Budgeted batched adversarial review|Budgeted review workflow security and publication|Bounded parallel adversarial review|Hosted budgeted adversarial review|Adversarial review large payload boundary' \
  -ginkgo.fail-on-empty
```

Observed: `Ran 165 of 3079 Specs in 144.963 seconds`,
`165 Passed | 0 Failed | 0 Pending | 2914 Skipped`.
Actual main-based discovery yielded disjoint, exhaustive partitions:
`3079 = 2937 base + 98 installation + 44 handoff`. The real scheduler criterion
passed separately; a discovery count does not mean all criteria were executed.

Negative controls observed seven failures when old serial/settings/deadline
behavior was restored, three failures when workers each received a private
budget, and one failure when a completed sibling was discarded. An independent
real HTTPS probe also reproduced cancellation at final completion reporting
success; the persistent regression now requires overall failure while retaining
completed coverage and zero-call authenticated resume. Cancellation with
incomplete coverage preserves the existing partial/nonzero result contract.

The selected model has not yet run through the new trusted-main client. Full
uncached hosted timing and automatic cache-save behavior still require that
prerequisite's human review and merge, followed by fresh exact-head execution.
