# ADR 0131: reduce qualification latency within free runner capacity

Status: user-authorized bounded performance work; independent acceptance precedes wiring

Date: 2026-11-02 UTC

Decision: 146

## Scope and baseline

After PR #161 merged, the user selected the three largest remaining opportunities:
balance macOS work, run duplicate quality stages once per OS, and parallelize
installation qualification. Preserve all logical groups, behavioral criteria,
native platforms, race instrumentation, fresh execution, failure propagation,
process deadlines and the existing 25-minute suite / 30-minute job allowances.
No paid runners, runtime changes, release defaults, dependencies or tool upgrades.

Successful run 41370385630 took 2,036 seconds. Its ten macOS jobs consumed 6,281
runner-seconds; the 1,225-second base job started 810 seconds after creation.
The later final PR #161 run 41385426253 took 1,606 seconds, demonstrating existing
runner variability before this change. Retain both baselines. An earlier Linux
installer timeout and macOS cancellation observation remain unexplained; passing
qualification or this scheduling change must not be described as their repair.

## Five execution lanes per OS

Use two job definitions with five execution lanes on each existing OS. Keep the
base job identifier and the four retained primary-group check names. The other
five logical groups become a second invocation within these jobs; no group is
removed. The repository's required status check is `selftest`, as read on
2026-11-02; this change does not modify repository protection settings.

| Primary group | Second group |
| --- | --- |
| base | none |
| release-handoff | default-init |
| local-upgrade | local-init |
| historical-recipe | default-upgrade |
| installation | local-recovery, then source-entrypoints |

The initial five pairs minimized old-duration estimates, but hosted run
41392210314 exhausted the base job's allowance. Keep base on its own and place
source-entrypoints after installation/recovery, which finished in 593 seconds.
This revised allocation requires hosted qualification; it is not a measured
speedup. Five macOS jobs avoid this workflow's own second wave; other workflows
still compete for capacity. Retain both OS values, fail-fast false, PR-only
supersession, immutable pins and full history; creation order is not priority.

Attempt all assigned groups after a group failure and return nonzero if any
group fails. Cancellation remains cancellation. Keep independent JSON/JUnit
paths and always-attempted per-group artifact uploads, including installation.
Each group retains its own suite deadline; the shared job allowance is unchanged.
Hosted measurements must determine whether these pairs retain adequate headroom.

## Quality ownership and local compatibility

Keep `make go-runtime-source-check` as the complete local entrypoint with all
existing formatting, shell syntax, dialect, vet, candidate build, lint, security
and vulnerability checks. Add `go-runtime-source-tests` as an explicitly scoped
qualification-only entrypoint sharing the same dispatch recipe. It must retain
source/group/worker validation, baseline checks, native package selection and
all behavioral controls, while omitting quality stages and candidate-build work
already owned by the complete check.

In CI, the base invocation owns the complete check once per OS. The other nine
logical groups use the test-only target. Install/restore quality executables only
for that quality-owning job; preserve exact tool pins and cold/warm cache behavior.
A quality failure must fail that job and must not be hidden by the second group.
Do not use a user-controlled skip-quality variable or cache successful test results.
Invalid group or worker settings still fail before qualification or quality work.

## Installation parallelism

Route installation through the same pinned Ginkgo CLI and resource-selected base
worker budget as base/source qualification. Preserve its exact family selector
and release-handoff exclusion, fresh race execution, empty-selection rejection,
literal report paths and nonzero worker-result propagation.

Audit fixtures before parallel execution. Preserve genuinely sequential journeys
and immutable-source independence. If a large ordered container only groups
independent scenarios for shared setup, split it into bounded independently
prepared containers with the same full criterion names. Never share mutable
projects or shorten cleanup lifetime; every worker owns its preparation and
each scenario owns its cloned project. Retain every existing assertion.
Measure serial and parallel execution before claiming an improvement.

## Qualification

An independent author must observe the old wiring fail the new dispatch, quality
ownership and lane contract. Extend existing scheduling criteria where appropriate;
preserve all 3,229 behavioral criterion names and independently discover the actual
registry and ten disjoint group populations. The installation group contains 98
criteria at this baseline. Exercise actual one/two-worker installation outcomes
and compare criterion identities, outcomes and worker assignments.

Execute the actual workflow shell recipes against owned command witnesses for
both cache states, all lane/group pairs, selected worker limits, quality failures,
first/second-group failures and empty selections. A string-only YAML check is not
sufficient. Preserve all original control coverage when refactoring these proofs.
Run required quality gates and the review diamond. Record actual hosted timings
and setup-cache differences; forecast ranges are not measured speedups.
All source builds and Make dispatch occur in private checkouts, preserving the
operator's primary developer binary and unrelated local files.

## Authoritative contracts consulted

Fetched 2026-11-02 UTC; no version change is proposed:

- https://docs.github.com/en/actions/reference/limits — Free capacity is twenty
  jobs overall, including at most five standard macOS jobs.
- https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax
  — matrix inclusion, failure handling, job timeouts and artifact-step conditions.
- https://onsi.github.io/ginkgo/#spec-parallelization — worker process isolation.
- https://onsi.github.io/ginkgo/#ordered-containers — ordered groups remain on one worker.

## Hosted qualification repair

Observed 2026-11-02 in run 41392210314: total elapsed 1,943 seconds, versus the
previous successful 1,606 seconds. macOS base acceptance took 1,383 seconds on
two workers, then source-entrypoints was cancelled at the job limit. A metrics
cancellation criterion read an empty PID file after creation but before writing.
Two default-upgrade preparations exhausted their existing 90-second image budget
during archive production. A local-upgrade helper exceeded its deadline while
capturing a selected worker; an untimestamped completion message does not prove
whether completion preceded the deadline or whether only pipe draining remained.

Repair fixture readiness by observing a complete positive PID, retaining both
signal paths and child-disappearance/publication assertions. Prove the empty-file
window deterministically rather than adding a sleep or increasing an allowance.

Default consumer fixtures may reuse their own immutable selected binary/catalog
across version-labelled images, using the existing independent image oracle.
The current production compiler has no version-specific linker substitutions;
metadata carries version/revision/target. Preserve distinct invoker/selected code,
actual committed source and baseline assets, all metadata/digest checks, private
scenario surfaces and container cleanup. Keep one combined 90-second first-image
budget and report preparation phases. Fast compression is permitted for fixture
archives; production packaging and its dedicated race qualification stay intact.

The revised five-job allocation has one base group, three two-group lanes, and
one installation/recovery/source-entrypoints lane. Independently qualify all
first/second/third failure and empty-selection paths, group-specific artifacts,
quality ownership, and the unchanged complete local target. All 3,229 criterion
identities and ten logical groups remain required. Do not increase deadlines,
retry failed criteria, suppress failures or claim unexplained runtime repair.

If local-upgrade reproduces, separate deadline, leader exit and capture completion
observations before changing production ownership. A fixture timeout must retain
its failure and clean its owned children; a printed completion line alone is not
an exit receipt. Hosted failures remain open until the affected paths pass on the
new head and the complete workflow supplies actual timing/headroom evidence.

Move the source-validation families `Go source qualification scheduling`,
`Go source qualification worker execution`, `Go source qualification workflow
partition` and `Native initializer copy inventory` from base into the existing
source-entrypoints group. Their 20 criteria consumed 353 aggregate criterion
seconds in the failing macOS run; that sum is not elapsed-time savings. This
gives the largest job more headroom while keeping these source checks together.
Retain exact anchored family matching, boundary-negative selector cases and
independent discovery. Preserve every old identity, with two additional process
helper ownership proofs: 3,231 total criteria, base 2,978, source-entrypoints 45,
and all other group populations unchanged. Qualify this selector change RED
before changing Make, then demonstrate a disjoint and complete native registry.
