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

## Interruption-fixture child identity

The final-head model review of checkpoint `8f8bbe8` identified a separate
test-only race in the deliberate CI transaction interruption helper. Its
concurrent `Cmd.Wait` could reap the owner between a publication witness and a
numeric PID/group signal. A buffered exit notification is not a reservation of
that identity, and a fresh liveness probe would retain the same race.

Keep the freshly started direct child unreaped while any interruption or
cleanup signal remains possible. Observe terminal state without consuming exit
status, using the existing platform-specific observation boundary. Complete
group termination before irreversibly entering the one-shot bounded wait;
repeated cleanup must never signal after reaping has begun. Preserve early-exit
diagnostics, cancellation cleanup, actual stopped-state observation, journal
immutability after termination and the existing 90-second operation deadline.
Independent controls must exercise early exit, a stopped live child, cancellation
and repeated cleanup. This is a confirmed source-level ownership race; no
recycled PID or collateral signal was observed during qualification.

## Remaining local-upgrade fixture owners

Hosted run `43426935557` qualified 31 of 32 Go jobs at checkpoint `8f8bbe8`.
The macOS direct-installed exclusive-admission criterion reached the selected
transaction, then exhausted its 90-second allowance. Its report confirmed child
exit, no ownership uncertainty, and retained reverse-recovery advice. The
transaction's exact internal phase was not captured; do not claim a diagnosed
lock deadlock, compiler failure or measured CPU contention.

The nine local-upgrade criteria already have two independent ordered fixture
owners: two legacy/predecessor-integrity criteria and seven routing/compiler/
admission criteria. Their worker chains took 795.105 and 673.811 seconds during
that run. Prior successful runs accumulated 1,487.235 and 1,553.701 seconds;
serializing the combined family would leave no reliable 25-minute headroom.

Select the existing two-criterion owner as `local-upgrade-predecessor`; keep the
other seven in `local-upgrade`. Run both consumers with one worker and preserve
the existing isolated two-case `local-upgrade-go` family. No new scenario or
fixture setup is duplicated by this split. Preserve exact criterion names and
the full `all` union, the real public installed `upgrade --local` route, all
compiler-count assertions and existing 90-second/25-minute/30-minute limits.
Update positive, negative, empty-selection and workflow scheduling controls
together. The independent interruption-control criterion raises the total
census from 3467 to 3468; the partition change does not change that total.

## Interruption-control platform evidence

Fetched on 2026-11-13 UTC: [Go's Darwin wait implementation](https://go.dev/src/os/wait_unimp.go)
documents the signal/reap identity race, and [Apple's process states](https://github.com/apple-oss-distributions/xnu/blob/main/bsd/sys/proc.h#L164)
define the unreaped zombie state as `SZOMB = 5`. The retained-child boundary
avoids relying on a concurrent `Process.Signal`/`Wait` pair for numeric identity.

The independent natural-exit control observed Darwin returning `EPERM` for the
group signal while the owned child was still unreaped; the subsequent join
returned its original status 23. Preserve that error in the helper result and
retain borrowed inputs at the transaction caller. Only this natural-exit
Darwin control permits that specific error; stopped and canceled live-owner
controls still require successful group termination. Do not convert an
unconfirmed signal into a claimed cleanup success.

## Held-source-child cleanup review

The complete review of checkpoint `cb79827` identified related cleanup hazards
in the source launcher's uncertainty evaluator. It released a held helper before
issuing `SIGCONT`, and could kill its launcher while another goroutine reaped
that launcher. Neither a finished-channel poll nor a new liveness probe reserves
the numeric identity. No PID reuse or collateral signal was observed.

Reuse the interruption owner's signaling/reaping phase boundary with an explicit
direct-process constructor for the evaluator's borrowed process group. Preserve
the existing isolated-group constructor and CI interruption semantics. A terminal
join must confirm exit without reaping under the phase mutex, close signaling,
and enter the sole bounded wait without issuing another signal. A live-child
join request must refuse without changing ownership; cancellation and failure
cleanup use the same serialized boundary. Never signal the evaluator's borrowed
group.

Complete launcher custody before releasing its held helper. Resume only a helper
that this evaluator actually stopped, while its release pipe is still held, then
close that pipe exactly once. Perform no mutating PID operation after release.
Missing or malformed readiness, failed resume/release, uncertain launcher join
or unconfirmed helper absence retain the fixture before assertions fail. Copy
the existing qualified helper bytes into this fixture's owned directory so
retention also preserves its actual executable; do not recompile or change the
shared suite's cleanup policy. Preserve the original signal, capture-isolation,
publication and absence assertions and all existing deadlines. Extend existing
independent criteria for direct ownership and release ordering; do not change
the census to manufacture coverage.

The bounded audit of both source-launcher fixture files also found the same
concurrent-wait/late-signal pattern in cancellation observation, compiler
admission, absent compiler groups and prior background jobs. Apply the shared
direct owner to those four paths together, preserving their existing process
attributes and observation deadlines. Consolidate parent/descendant cleanup and
retain evidence before failed absence assertions. Healthy compiler completion
must still release its compiler before the launcher can exit; reserve the
launcher identity throughout that release and close signaling before its sole
wait. Preserve actual launcher absence before releasing unrelated held children.

## Local-recovery phase diagnostics

Hosted macOS job `131973875574` at checkpoint `cb79827` timed out both local
recovery calls before publication admission. Reverse reached candidate
materialization; forward reported cancellation before `Complete`. The two
criteria already execute sequentially within one ordered owner. The acceptance
suite is race-instrumented, but that does not establish the build flags of its
child CLI; no child-race selector is configured in this workflow.

Keep the original public invoker, authority checks and deadlines while measuring
coarse preparation phases in the already instrumented selected-source fixture.
Record actual driver build metadata and preview duration, then selected Begin,
plan completion, target/predecessor staging and candidate materialization. These
are best-effort diagnostic receipts, never admission or readiness authority. Read
them into the report after the caller joins. No shipping hook or production
control is added, and no timeout, validation or acquisition work is removed.
Choose any further correction from that evidence rather than inferring race
overhead or host contention from a timeout alone.

The diagnostic run above preserved the original ninety-second context. The
subsequent test-budget decision is recorded separately in [ADR 0152](0152-phase-bounded-local-recovery-qualification.md); it supersedes only the two
recovery criteria's evaluator allowances, not runtime or source-fixture cleanup
limits.
