# Integrated installed adopter rehearsal

Contract: [ADR 0156](../adr/0156-integrated-adopter-rehearsal.md), Decision 172.
R4 joins the installed boundaries after PR #182. Local and hosted source
qualification passed; PR #183 merged on 2026-11-15 as
`509d4bbc01743fa3900247273d082171e1bf3a18`.

## What this evidence covers

The rehearsal packages current factory source and invokes the ordinary installer
with an explicit stable selection supplied by a local release fixture. It uses
real public Go commands, genuine immutable v0.1.6 initialization, actual Git
commits/clones, generated workflows and the documented full-check hook.

GitHub release/attestation transport and model responses are test fixtures. The
installer, native acquisition, verification policy, installation transaction,
runtime checks and review client remain real. A passing result establishes the
connected source behavior; it does not establish that the fixture release exists
on GitHub or that a published asset has passed live authentication.

| Journey | Required observation | Result |
|---|---|---|
| Fresh project | Ordinary initialization, explicit review opt-in, committed public assets | Local PASS |
| Supported v0.1.6 migration | Ordinary preview and confirmation, preserved user files/settings, classified retirement and inert ignored originals | Local PASS |
| Clean ordinary CI | Generated preparation, real checks, three harness evaluations, mock golden evaluation and propagated application failure | Local PASS |
| Clean enabled review | Generated selector, exact pinned installer, real standalone acquisition and public review with local provider transport | Local PASS |
| Local full-check hook | Real topic push succeeds; application failure prevents remote update | Local PASS |
| Recovery and retention | Exact rollback plus retained complete interruption/retention families | Local and hosted PASS; initial timeout/repeat noted below |

The initialized-CI group owns the two connected journeys. Existing CI migration,
release preparation, interruption and retention groups retain their detailed
failure controls. No check is removed merely because a longer journey exercises
some of the same code. All generated commands run from the committed project;
the rehearsal must not repair its configuration, Makefile or harness files after
cloning to make the checks pass.

Default-off initialization and off/absent migration choices retain their existing
default-init and CI-migration controls. The connected migration preserves unknown
operator history. It does not put modern budget/loop state into a v0.1.6 rollback
success fixture: the existing state-compatibility tests require refusal when an
older controller cannot interpret that state. Historical Bash rollback health
uses its historical tools; compiler/Python prohibition applies to the Go phase.

## Local result

On 2026-11-15, the complete initialized-CI source gate exited 0 on darwin/arm64
with Go 1.27.4. The evaluator ran with race instrumentation:

```sh
make go-runtime-source-check GO_RUNTIME_TEST_GROUP=initialized-ci \
  GO_RUNTIME_REPORT_DIR=.factory/ci-mode-evidence/integrated-adopter/initialized-green
```

Output: `Ran 8 of 3501 Specs in 940.527 seconds`,
`SUCCESS! -- 8 Passed | 0 Failed`. Subsequent quality output was `0 issues.`,
`Issues : 0`, and `Your code is affected by 0 vulnerabilities.` The scanner
also reported ten module-level vulnerabilities whose affected code is not called;
this is not a claim that every required module has no advisory.

The two connected criteria used fixture version `v0.2.0`, source commit
`1feec9bf81178f77d144a24e430f179828c182a4` and executable SHA-256
`5f9dffd5e30c0329bf8c111bf0445000882d5177ec0ff8e04f3c233a1ef76e33`.
That is the isolated producer's actual source commit and digest, not a published
tag or a claim that the release exists. Detailed commands, phase statuses and
captured diagnostics are in the local Ginkgo report at the directory above.

The source gate includes all eight existing/new initialized-CI criteria; its
fresh connected criterion took 207.940 seconds and its migrated criterion took
119.634 seconds. The group total includes the other initializer/preview cases
and shared fixture setup. A later one-line review correction restores unchanged
cancellation behavior for non-JSON destinations; final focused and hosted checks
qualify that final source separately.

After that correction, the final focused command was:

```sh
go test -race -v -count=1 -timeout=25m ./acceptance \
  -ginkgo.focus='(Native orchestration sync|Go source qualification (scheduling|worker execution|workflow partition))' \
  -ginkgo.no-color -ginkgo.fail-on-empty \
  -ginkgo.json-report=../.factory/ci-mode-evidence/integrated-adopter/final-targeted.json
```

Output: `Ran 53 of 3501 Specs in 373.710 seconds`,
`SUCCESS! -- 53 Passed | 0 Failed`. This includes every native synchronization
criterion, exact discovery of all 3,501 criteria across disjoint CI partitions,
and worker execution/workflow partition controls. Subsequent hosted checks on
the final commit qualified the full matrix and connected journeys, as recorded below.

## Reproduce source qualification

Use the repository's qualified source-development toolchain and dependencies.
These are evaluator requirements, separate from the installed subprocesses,
whose environment rejects compiler and Python execution.

```sh
make go-runtime-source-check GO_RUNTIME_TEST_GROUP=initialized-ci \
  GO_RUNTIME_REPORT_DIR=.factory/qualification/integrated-adopter/initialized-ci
make go-runtime-source-check GO_RUNTIME_TEST_GROUP=ci-migration \
  GO_RUNTIME_REPORT_DIR=.factory/qualification/integrated-adopter/ci-migration
```

Keep machine idle sleep disabled only for the duration of a long local run when
needed; do not change runtime timeouts to compensate for a sleeping evaluator.
Hosted exact-head reports remain the cross-platform source qualification.

## Release gates remaining at R4

R4 closed with the reviewed merge and hosted evidence below. R5 publishes and qualifies RC4 on the
four supported native targets, using actual release authenticity. R6 records the
representative adopter pilot and operator acceptance. R7 performs stable
activation with the final dependency/reference audit. This rehearsal does not
change the public v0.1.6 default or create another release gate.

## Final hosted evidence and merge

[PR #183](https://github.com/anoop2811/software-factory-template/pull/183)
head `678f3a13dee941f94de0474e7cba9c64d9c2bd72` passed 46 current checks.
The merged revision above has the same tree. The
[Go qualification run](https://github.com/anoop2811/software-factory-template/actions/runs/43757256788)
ran all eight initialized-CI criteria successfully on Linux (221.064 seconds)
and macOS (642.411 seconds). The two new fresh/v0.1.6 journeys passed on both
platforms in the first attempt. The model review completed all eleven files and
seventeen fragments in 2m25s; every finding has a
[full-source response](https://github.com/anoop2811/software-factory-template/pull/183#issuecomment-6900760571).
Copilot could not review because the account quota was exhausted.

One separate existing macOS local Go-to-Go apply hit its 90-second fixture
watchdog in the first matrix attempt. Its process confirmed exit and retained
recovery evidence. The unchanged two-case local reproduction passed in
504.123 seconds; a single hosted job repeat passed in 478.785 seconds without
changing code or deadlines. The original overrun's underlying cause remains
unproven. Commands, exact results and both artifact identities are retained in
the [final qualification receipt](https://github.com/anoop2811/software-factory-template/pull/183#issuecomment-6901407780).
These are source results, not published RC4 or adopter-pilot evidence.
