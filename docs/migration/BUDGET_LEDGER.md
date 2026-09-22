# Go budget ledger and admission candidate

[ADR-0069](../adr/0069-go-budget-ledger-admission.md) defines the shared storage
and admission component for the G2 budget-controller conversion. It is based
on the merged Python controller, independently of native-execution PR #95.

## Integration boundary

The component reads existing schema-1 history, computes plans/reports and
performs locked admission, PID publication and finalization. Go and Python
coordinate through the same persistent `.factory/budget.lock` inode. Read-only
planning and reporting do not create storage. Unknown history fields and exact
integer counters remain intact.

This is internal source functionality. Public `factory budget` and `factory loop`
still use their installed controllers. No new command can reserve or launch an
agent through this candidate. Compiled acceptance drivers are test-only and are
not included in command discovery or release assets.

The future controller must combine initial read-only admission, local native
preflight, locked re-admission, reservation, PID publication before prompt
delivery, execution and final publication. An admission or publication error
does not authorize launch or an automatic retry. An ambiguous post-rename error
requires inspection because the reservation may already be visible.

## Qualification

The qualified environment is a trusted project namespace on a local supported
Linux/macOS filesystem with cooperating writers, flock, atomic rename and
file/directory sync. This is not protection against hostile same-user filesystem
replacement or a network-filesystem compatibility claim. History uses bounded
finite scalar JSON; the ADR defines all limits and deliberate tightenings.

Float sums are qualified against CPython 3.12's accumulation behavior. Earlier
Python rounding differences remain an activation question. Counter preservation
does not rely on float64 decoding. No Python interpreter is called by the Go
component; the immutable Python source is an acceptance oracle only.

## Remaining conversion

Controller execution/output, shell model/configuration integration, loop state
and fingerprint interoperability still need acceptance. Activation must include
safe installation, gitignored recovery copies, rollback and predecessor pruning.
No obsolete runtime is retired by this component alone. Completing this slice
does not finish the budget-controller acceptance package or the G2 release gate.

## Evidence

Local macOS evidence recorded on 2026-09-21. The independent evaluator authored
61 outside-in cases and five storage collaborator cases; production code was
written by a separate implementer. Correctness and security reviewers examined
the final component. These checks do not qualify installed controller behavior.

Initial behavioral RED, before substantive implementation:

```sh
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test ./acceptance -ginkgo.focus="G2 interoperable budget ledger" -ginkgo.no-color -ginkgo.succinct -count=1
```

The executed command redirected output to a local log. Its result was:

```text
Ran 3 of 815 Specs in 3.927 seconds
FAIL! -- 0 Passed | 3 Failed | 0 Pending | 812 Skipped
```

Additional regressions were independently observed failing before correction:
invalid Python numeric syntax, unnecessary cost aggregation, missing uncertainty
metadata, canceled empty-history reports, and contradictory pre-spawn exit data.
The first expanded race run also exposed two evaluator defects: an incorrectly
indented Python oracle branch and an eager read of a live stderr buffer. Both
fixtures were corrected without weakening assertions before this final run:

```sh
rtk proxy env FACTORY_AGENT_ROLE=spec-writer FACTORY_BUDGET_TEST_RACE=1 go test -race ./acceptance -ginkgo.focus="G2 (interoperable budget ledger|budget)" -ginkgo.no-color -ginkgo.succinct -count=1
```

```text
ok  github.com/anoop2811/software-factory-template/acceptance 25.749s
```

The environment switch compiles the temporary ledger driver with race
instrumentation, in addition to instrumenting the parent test process. Tests
exercise real Go/Python lock contention, simultaneous admission, exact history
round trips and process ownership using local fixtures; no paid agent executes.

Independent storage race run:

```sh
rtk proxy env FACTORY_AGENT_ROLE=reviewer go test -race ./internal/budget -count=1 -v
```

```text
5 Passed | 0 Failed
ok  github.com/anoop2811/software-factory-template/internal/budget 1.783s
```

Linux-target static analysis with the repository's pinned lint configuration:

```sh
rtk proxy env GOOS=linux GOARCH=amd64 /private/tmp/factory-quality-tools/golangci-lint run --config packs/go/.golangci.yml ./...
```

```text
0 issues.
```

Linux execution remains a CI qualification; cross-target lint alone is not a
Linux runtime result.


The repository gate ran through `make check` with the unchanged pinned quality
tools rebuilt for the configured Go toolchain:

```sh
rtk proxy env FACTORY_AGENT_ROLE=reviewer bash -c 'PATH="/private/tmp/factory-quality-tools:$PATH" make check > /private/tmp/factory-budget-check-final.log 2>&1; result=$?; tail -45 /private/tmp/factory-budget-check-final.log; exit "$result"'
```

Selected output from the Go source gate inside that command:

```text
go test -race -count=1 ./...
ok  github.com/anoop2811/software-factory-template/acceptance 303.568s
ok  github.com/anoop2811/software-factory-template/internal/budget 1.435s
0 issues.
Issues : 0
No vulnerabilities found.
```

The source gate also ran formatting, shell syntax, Ginkgo dialect enforcement,
`go vet` and the candidate binary build. No live provider reliability or installed
runtime cutover is inferred from this evidence.

`make check` exited 0. Its unchanged shell suites reported:

```text
selftest: 217 passed, 0 failed, 0 skipped
budget: 32 passed, 0 failed
loop: 40 passed, 0 failed
adversarial-review: 53 passed, 0 failed
shared-script-enforcement: all harness adapters use shared scripts correctly
copy-manifest-check: every unconditional install-copy target is git-tracked
gate-instrumentation-check: all 14 blocking gate(s) can report a block
workflow-lint: OK (2 recipe(s))
```

`rtk proxy env FACTORY_AGENT_ROLE=reviewer make check-drift` exited 0 and left
no adapter changes. The citation gate skips when the configured prefix is empty;
ADR citations were checked against the actual numbered source separately.
