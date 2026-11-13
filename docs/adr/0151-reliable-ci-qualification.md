# ADR 0151: coherent CI qualification ownership and budgets

Status: Accepted for implementation; qualification pending.

Decision: 167, 2026-11-13 UTC.

## Context

The user requested a comprehensive correction after repeated PR #180 CI failures,
rather than additional isolated patches. Audit run `43356159724` and predecessor
runs at `6d64d6d` and `2eb4feb` identify distinct causes that must not be conflated.

The latest run had 35 successful checks, four failed checks and one cancellation.
Its macOS base tests passed before the overall job expired during quality work.
Other failures occurred during incidental fixture packaging, non-Git evaluation
preparation, a real source initializer, historical writer preparation and the
aggregate retention schedule. The reports do not establish leaked descendants,
host contention or the precise inner stage of every failed subprocess.

The 28 Go jobs repeatedly compile acceptance and run global quality checks.
Ginkgo starts its CLI timeout before compilation: the handoff suite received
about 1331 seconds of its nominal 1500-second allowance. All latest quality-tool
installs were cache hits; acquisition did not cause those failures. Earlier cold
installs took 65–144 seconds. The cache key unnecessarily includes the complete
workflow, so a routing change invalidates unchanged tools.

macOS has limited hosted concurrency. The final handoff job waited over 30 minutes
before starting, and the run consumed about 200.53 macOS runner-minutes. Keep
execution cost, queueing, compilation and suite time separate when reporting
improvement. The under-20-minute requirement applies to adversarial review, not
an invented total-runtime target for the complete Go workflow.

## Shared preparation and execution

Keep the existing `go-runtime` check identity as a preparation job for each OS.
It must execute package tests, formatting, dialect checks, vet, candidate build,
lint and security checks on every run. It also prepares the race-instrumented
acceptance executable and the pinned Ginkgo runner once for that platform.

Acceptance jobs depend on successful preparation and execute the precompiled
suite. They retain the real Go, Python and Git tools required by fixture journeys,
the existing resource-aware worker caps, race instrumentation, fail-on-empty,
literal group selection, nonzero status propagation and per-group reports.
Installation remains serial but gains a retained JSON/JUnit report. Global quality
checks are not repeated in every acceptance job and cannot be bypassed by a cache
hit. Preserve local `all`, `base` and source-compilation behavior; use explicit
CI preparation and consumer targets rather than a general skip-quality option.

Ginkgo derives a precompiled suite's working directory from its binary location.
The suite therefore lives at `acceptance/qualification.test`, already covered by
the repository's `*.test` ignore rule. The runner, receipt and preparation
diagnostics live under the ignored `.factory/qualification/prepared-v1/` directory.
These are repository qualification tools, not new installed factory commands.

Use a private Cobra command in `internal/qualification/cmd/prepare-suite` with
`prepare` and read-only `verify` operations. Preparation uses existing native
process supervision for compiler children, writes diagnostics on failure, and
publishes an admissible receipt only after both executables are complete.
Verification must not execute cached programs or silently rebuild them.

The versioned receipt binds the actual checked-out commit, module inputs,
platform, Go/Ginkgo identities, race configuration and executable digests.
Reject missing or malformed receipts, changed bytes, mismatched identity,
unexpected placement, unsafe file types and unusable executable modes before
launching the suite. A PR's checked-out merge commit is distinct from its head;
bind the actual checkout and workflow SHA consistently.

Build with `GOENV=off`, `GOTOOLCHAIN=local`, `GOWORK=off`, empty `GOFLAGS`,
native `GOOS`/`GOARCH`, explicit readonly module selection and `CGO_ENABLED=1`
for the race suite. Empty `GOFLAGS` alone does not override persisted Go
environment settings. Inspect each executable's actual Go build information
for the compiler, platform, module and race identities before accepting it.
Keep the hosted PATH, C compiler, caches and module proxy configuration. This
receipt qualifies an image for the same workflow run and platform; it does not
claim bit reproducibility across arbitrary C compiler or host environments.

## Transport and cost

Use an exact same-run cache key bound to workflow run ID, checked-out commit,
platform, compiler/Ginkgo configuration and schema. Require an exact hit; partial
matches, missing entries or invalid receipts must fail closed. Do not add restore
prefixes or a consumer-side rebuild fallback. Cached content is executable input,
never a cached test result.

Do not key the image to the consumer's current run attempt. A failed-job rerun
may reuse its successful producer from an earlier attempt in the same immutable
run. Record producer attempt as provenance, but allow this qualified reuse.
Rerunning a producer always reruns its package and quality checks; a valid image
for the exact same run and source may be reused. Other runs or commits cannot
match. Consumer tests always execute again.

Keep standard hosted runners and existing cache limits. This avoids adding
retained executable artifacts to the separate artifact-storage allowance.
The repository cache usage observed on 2026-11-13 UTC was 4,284,543,002 bytes
across 69 entries; record resulting image sizes and usage rather than promising
unmeasured storage or timing savings. Do not change spending limits or permissions.

Key quality-tool caching by actual compiler, platform, analyzer versions and
security module inputs. A routing-only change must not change tool identity;
changes to relevant pins must invalidate it. All restored tools still execute.

## Consistent fixture boundaries

Route incidental full-installation image production through one supervised native
producer, including the two remaining direct fixture-only `packaging.Build`
sites and their consumers. Preserve the selected runtime compiler separately
from the qualified compiler that builds the producer. Preserve source, revision,
target, output and existing 90-second preparation limits. Share immutable image
bytes only; keep fresh caches and genuinely initialized adopters.

Keep the six direct packaging API/cancellation tests in-process. Keep independent
archive/manifest oracles and distinct retention runtime build IDs. Substituting
the implementation under test for those oracles would reduce coverage.

Complete deterministic non-Git discovery in equivalent evaluation fixtures.
Do not replace qualified real Git where Git selection or source build metadata is
part of the criterion. Apply pre-clone maintenance configuration only to owned
setup repositories whose complete snapshots require quiescence, preserving
adversarial Git inputs.

Retain the historical writer's documented recursion guard only for marker
preparation, with genuine initial and restored legacy qualification unchanged.
Keep ADR 0148's disjoint handoff/retention selections and independent retention
owners, preserving every existing criterion identity and actual upgrade journey.

## Process ownership

Centralize a tested lifecycle contract, not a universal environment policy.
Successful mutable initialization, upgrade and recovery journeys must declare
owned and borrowed roots, use cooperative supervision and retain evidence when
cleanup is unconfirmed. Compiler/setup work uses existing setup supervision.
Preserve each caller's interpreter, literal arguments, stdin and environment.

Do not blindly replace expected pre-acquisition refusals or deliberate signal,
PTY and blocked-input tests. Their oracle must observe product cleanup before any
evaluator fallback. Ordinary expected refusals must not accumulate retained
fixtures. A bounded pipe wait is not proof that all descendants have settled.

Before broader adoption, independently exercise the fixture helper with normal
and nonzero exits, inherited-stream descendants, cooperative nested cleanup,
forced uncertainty, unrelated-process survival and actual fixture deletion versus
retention. Confirmed nonzero status must reach existing preservation assertions.

## Required evidence

Use independent failing controls before implementation. Cover failed preparation,
missing or mismatched cached input, changed executable bytes, wrong working
directory/modes, empty selection, test failure, both cache states and partial
workflow reruns. Prove global checks run once per platform and every acceptance
criterion remains in exactly one intended selection.

Retain existing criterion identities. Additional direct controls must be counted
explicitly; unchanged counts alone do not prove unchanged work. Require complete
final-head Linux and macOS qualification, including base, installation, handoff
and retention, with completed quality checks and explained platform skips.
Inspect setup, compile, execution and job margins on cold and warm observations.
Do not describe a job with only seconds remaining as having established headroom.

The change is not ready merely because targeted checks pass. Address and reply to
new review findings, retain the adversarial review's existing time budget, and
leave protected changes for human review and merge.

The explicit custody adoption in this correction covers successful mutators in
the audited source initialization, local recovery, release CI and release handoff
families. Include initialization, confirmed application, rollback and confirmed
recovery where those families exercise them. Keep caller-specific deadlines and
place evaluator stream files outside adopter snapshots. Shared wrappers that
also serve expected refusals are not a reason to change those refusal oracles.
This scope does not assert that every subprocess helper in the repository has
been converted. Track the concrete call sites and qualify each affected family.

## Authoritative sources

Fetched on 2026-11-13 UTC:

- [Ginkgo precompiled suites](https://onsi.github.io/ginkgo/#precompiling-suites)
  and [pinned runner implementation](https://github.com/onsi/ginkgo/blob/v2.32.1/ginkgo/run/run_command.go).
- [Go command process and pipe behavior](https://pkg.go.dev/os/exec#Cmd).
- [GitHub dependency-cache semantics and eviction](https://docs.github.com/en/actions/reference/workflows-and-actions/dependency-caching).
- [GitHub Actions billing and separate cache allowance](https://docs.github.com/en/billing/concepts/product-billing/github-actions).

These sources establish interfaces and constraints; measured repository outcomes
remain the evidence for performance and reliability claims.
