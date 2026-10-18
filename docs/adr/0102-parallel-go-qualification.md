# ADR 0102: parallel Go qualification with independent fixtures

- Status: user-authorized implementation contract; acceptance evidence precedes implementation
- Date: 2026-10-18 UTC
- Decision: 101

## Scope and baseline

The user authorized performance priorities 1, 2 and 3 after merging PR #128: two
Ginkgo workers for base acceptance, lighter component fixtures, and independent
release-handoff scenario groups. Keep all acceptance behavior, platform coverage,
source qualification, race checks, failure propagation and existing timeouts.
Do not expand this change into compiler-cache redesign, quality-stage removal,
paid runners, or replacement of historical compatibility oracles.

The completed [PR #128 run 39102958496](https://github.com/anoop2811/software-factory-template/actions/runs/39102958496) provides the baseline: macOS base job 1,451 s,
acceptance 1,328.933 s; Linux base job 919 s, acceptance 702.777 s. The macOS handoff job
was 1,052 s. These are different runner executions, not a controlled OS benchmark.
The new implementation must publish actual timings rather than promise a ratio.

## Worker execution and reports

Use the Ginkgo CLI already selected by go.mod, v2.32.1 (released 2026-08-10),
confirmed at https://github.com/onsi/ginkgo/releases/tag/v2.32.1 on 2026-10-18 UTC.
Do not upgrade the module or introduce another dependency. Ginkgo workers are
separate processes; ordinary Go test package parallelism does not distribute
specs within the single acceptance suite. Run base acceptance with exactly two
workers in CI. Run every non-acceptance Go package as before, retaining its
race-enabled fresh execution and empty-selection safeguards. Bound local worker
selection to one or two for a serial comparison; reject invalid values before work.

Run release-handoff acceptance with two workers after isolating its scenarios.
The installation group retains its current execution policy. Preserve all three
group selectors and existing check names on Linux and macOS. Keep the complete
local Make entrypoint and all vet, formatting, build, lint and security stages.
Do not count an omitted package, empty suite, duplicate criterion or skipped
worker failure as success. Continue attempting other selected groups after a
failure and propagate the actual nonzero result.

Ginkgo execution must keep race instrumentation on the test harness and fresh
count-one behavior; this does not imply every child executable is race built.
Retain the 25-minute suite/hard test allowance and 30-minute CI job allowance.
Use the official CLI's supported arguments, confirmed by the pinned runner's help
before wiring them. Collect machine-readable per-spec JSON and JUnit reports for
base and handoff, with independent paths and always-attempted artifact retention.
Reports must include actual identities, outcome and timings. They are evidence,
not an execution authority or substitute for failure propagation. No paid calls.

## Fixture ownership and preparation

Split bare private directory setup from fixtures that need a candidate CLI and
legacy launcher. Component-only callers should install only their actual owned
driver and assets. Audit each changed caller: retain complete CLI fixtures for
behavior that executes the dispatcher or legacy oracle. Do not share mutable
files, hardlink binaries, relax modes, or change inode/custody tests. Lazy component
build caches that already exist remain per process; do not rebuild them per spec.

Split handoff into independently prepared scenario groups. Use group-scoped
BeforeAll/cleanup for immutable authenticated test images, compiled predecessor
bytes and pinned legacy templates; keep their preparation shared in one helper.
Limited duplicate preparation across two workers is acceptable and must appear
in measured timings. Preserve every existing full criterion name; identical
container descriptions with distinct group labels can retain those identities.
Prepare distinct project, transport, trace, journal and recovery paths per scenario.
Never execute setup during Ginkgo tree construction or dry discovery.
Preserve independent target builds and distinct predecessor build identities.
Do not allow another worker to clean a live fixture. No production trust boundary,
installation engine, migration order or publication guarantee changes.

Remove whole-container ordering only where specs depend solely on immutable setup
and fresh per-spec fixtures. Keep every assertion and each complete interruption,
observation, recovery and rollback sequence within its original criterion. A
failure in one independent scenario must not hide the others. Independent JSON
comparison must show every former handoff criterion once, without lost controls.

## Qualification and measurement

An independent spec author owns scheduler/coverage criteria. Observe RED for the
new worker/reporting contract before wiring the runner. Prove one/two-worker
configuration, invalid-worker refusal, exact group selection, no lost internal
packages, failure propagation and nonempty coverage. Preserve the acceptance
census, updating it only for independently justified new criteria. Exercise the
actual runner on multiple worker processes; strings in a workflow are insufficient.

Use the same independently declared behavioral identities to compare serial and
parallel handoff runs, including both legacy profiles, Go predecessor identity,
read-only previews, private backups, fresh consent and forward/reverse recovery.
Run affected component families after changing fixtures. Run the configured pack
quality checks and review through workflows/review-diamond.md. Hosted Linux/macOS
runs must establish actual final-head results and timings before a speedup claim.

## Authoritative framework contracts

- https://onsi.github.io/ginkgo/#spec-parallelization — separate worker processes.
- https://onsi.github.io/ginkgo/#ordered-containers — an ordered group stays on one worker.
- https://onsi.github.io/ginkgo/#generating-machine-readable-reports — JSON/JUnit outcomes and timelines.

These contracts were fetched 2026-10-18 UTC. Existing application, task and process
watchdog limits remain. This change optimizes qualification and grants no new
runtime, cleanup, release or migration authority.

CI retains reports with the already used actions/upload-artifact v7.0.2,
commit `cf430e030ddbb5b0abf93d22962f4752f3646cd9` (released 2026-10-07).
Its official release and tag identity were fetched 2026-10-18 UTC at
https://github.com/actions/upload-artifact/releases/tag/v7.0.2 before wiring.
Missing reports warn if setup failed before execution; the original runner status
still fails the job. Successful base and handoff runs must produce their reports.

## Hosted qualification failure diagnostics

The first head's hosted macOS handoff report retained two failures with only
`native process ownership is unconfirmed`, during installed-proof Git fixture
preparation. Linux and the full local macOS handoff passed. The report does not
identify a signal, reap, pipe-drain or terminal-group failure, so no root cause
or safe process-policy correction is established by that evidence.

Before another diagnostic qualification, preserve the fixed OwnershipError.Error
message, typed identity and PID. Add a safe diagnostic accessor recording only
the failed cleanup phase, PID, numeric/unknown syscall classifications, reap and
drain booleans, and the last group observation. Do not expose command arguments,
paths, environment, child output or arbitrary underlying error messages. At the
installed-proof boundary, wrap this safe evidence and the fixed gate/setup step
while retaining errors.As access to OwnershipError.

Retain the exact short-circuit order, five-second shared cleanup deadline,
20-millisecond group polling, ESRCH requirement and signal-error refusal. Do not
retry commands, ignore EPERM, add watchdog headroom or treat a rerun as root-cause
closure. Independent fault criteria must establish diagnostic accuracy and
privacy before implementation. These diagnostics grant no execution, cleanup,
publication or migration authority.
