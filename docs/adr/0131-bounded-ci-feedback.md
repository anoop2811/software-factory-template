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
| base | source-entrypoints |
| release-handoff | default-init |
| local-upgrade | local-init |
| historical-recipe | default-upgrade |
| installation | local-recovery |

These fixed pairs minimize the longest paired duration across both retained
baselines (1,597 seconds before removing duplicated setup/quality), and reduce this workflow from
ten to five macOS jobs, avoiding its own second wave of queued jobs. They do not
reserve account capacity or guarantee priority over other workflows. Retain both
OS matrix values, fail-fast false, existing PR-only supersession, immutable
action pins and full-history source checkout. Do not depend on matrix creation
order as a scheduling guarantee.

Attempt both groups after a group failure and return a nonzero lane status if
either fails. Cancellation remains cancellation. Keep independent JSON/JUnit
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
