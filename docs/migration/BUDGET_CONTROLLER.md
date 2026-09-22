# Go budget execution controller candidate

[ADR-0070](../adr/0070-go-budget-execution-controller.md) composes the existing
Go budget ledger, native supervisor and usage parser. It retains initial read-only
admission, local preflight, locked re-admission, ownership publication before
prompt delivery, and durable finalization before an answer can be returned.

## Delivery boundary

This is an internal candidate exercised by temporary compiled drivers. Public
`factory budget` and `factory loop` still route to their installed controllers.
No new launch command, installer activation or release asset is introduced.
Public live output timing, text/JSON rendering, shell model resolution, loop
checkpoint interoperability and safe retirement remain separate obligations.

A successful merge does not move an adopter's existing scripts or history.
Activation still requires transactional delivery, gitignored local recovery
copies, rollback and retention cleanup. This slice advances the existing budget
controller work package without declaring that whole package complete.

## Go design

Runner coordinates the operation; Ledger owns state and native owns processes.
The controller reuses the canonical admission, parsing and lifecycle rules.
Concrete types and small private per-instance collaborators keep fault testing
possible without an exported mocking framework or global mutable test hooks.
Context deadlines bound execution; a separate bounded cleanup context allows
finalization after cancellation. No new dependencies or version pins are needed.

Any future additional library must justify its maintenance cost and have a
verified permissive open-source license, including relevant transitive code.
The existing Cobra and Ginkgo/Gomega stack remains in place.

## Evidence

The independent evaluator observed the initial five behavioral cases failing
against the compiling stub before substantive implementation:

```sh
rtk proxy env FACTORY_AGENT_ROLE=spec-writer bash -c 'go test ./acceptance -ginkgo.focus="G2 composed budget controller" -ginkgo.no-color -ginkgo.succinct -count=1 > /private/tmp/factory-controller-core-red.log 2>&1; result=$?; tail -24 /private/tmp/factory-controller-core-red.log; exit "$result"'
```

```text
Ran 5 of 958 Specs in 1.557 seconds
FAIL! -- 0 Passed | 5 Failed | 0 Pending | 953 Skipped
```

These cases cover disabled no-effect behavior, preflight refusal and durable
PID publication before prompt delivery for Codex, Claude and OpenCode.
No installed runtime or upgrade claim follows from these source-only tests.


## Composition regressions

Independent failure tests exposed two integration issues before their corrections:
an uncertain native timeout lost its timeout label, and preliminary unlocked
history reads rejected cooperating atomic replacements. The latter can make a
contending controller return a storage error despite a valid winning reservation.
The ADR limits replacement retries to read-only snapshot acquisition; it does
not authorize retrying an admission or a publication.

The controller suite uses explicit structured-result expectations and fake native
CLIs. It does not claim a new Python run-controller output differential oracle;
rendering is deferred. Existing ledger/native suites retain their immutable
baseline comparisons for the reused behavior.


## Final focused evidence

Local macOS evidence recorded 2026-09-22 UTC. The evaluator authored 23
outside-in cases and 28 internal controller cases independently of production.
The compiled controller driver and its fake harness processes were themselves
race-instrumented, not only the parent acceptance test process:

```sh
rtk proxy env FACTORY_AGENT_ROLE=spec-writer FACTORY_CONTROLLER_TEST_RACE=1 bash -c 'go test -race ./acceptance -ginkgo.focus="G2 composed budget controller" -ginkgo.no-color -ginkgo.succinct -count=1 > /private/tmp/factory-controller-final-race.log 2>&1; result=$?; tail -28 /private/tmp/factory-controller-final-race.log; exit "$result"'
```

```text
ok  github.com/anoop2811/software-factory-template/acceptance 16.365s
```

```sh
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test -race ./internal/budget -ginkgo.focus="Budget controller" -ginkgo.no-color -ginkgo.succinct -count=1
```

```text
ok  github.com/anoop2811/software-factory-template/internal/budget 7.250s
```

The internal command was executed with output redirected to a local log. A
subsequent test-only lint correction asserts that deferred flock unlock succeeds;
the affected case was rerun with its final assertion:

```sh
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test -race ./internal/budget -ginkgo.focus='bounds finalization lock waiting' -ginkgo.no-color -count=1
```

```text
ok  github.com/anoop2811/software-factory-template/internal/budget 6.408s
```

Regression RED evidence before the corresponding corrections:

```sh
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test ./internal/budget -ginkgo.focus='retains timeout uncertainty' -ginkgo.no-color -count=1
```

This ran one case: zero passed, one failed; actual `launch_error`, expected
`timeout`. The snapshot acquisition case likewise failed with unsafe-storage:

```sh
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test ./internal/budget -ginkgo.focus='reacquires a validated' -ginkgo.no-color -count=1
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test ./internal/budget -ginkgo.focus='Budget controller snapshot' -ginkgo.no-color -ginkgo.succinct -count=1
```

The first ran one case and failed; the second ran eight, with five passed and
three failed (both unlink windows and the retry bound). Generic open failures,
malformed JSON, symlink/hardlink refusal and cancellation already passed. The
correction did not weaken those assertions. Independent security review reran
all nine snapshot cases under the race detector:

```sh
rtk proxy env FACTORY_AGENT_ROLE=reviewer go test -race ./internal/budget -count=1 -v -ginkgo.focus='snapshot' -ginkgo.no-color
```

```text
9 Passed | 0 Failed
ok  github.com/anoop2811/software-factory-template/internal/budget 1.331s
```

Earlier focused checks passed while a subsequent full run reproduced another
admission error. The ordinary contention qualification below addresses that
separate lock-creation defect.
Linux runtime execution and installed delivery are not inferred from macOS tests.


## Repository checks

The first `make check` run passed its shell suites, then exposed the real
contention regression in the Go acceptance suite (962 passed, one failed, 13
skipped). It was not counted as a passing gate. After correction, the
full Go source gate was rerun separately. The already-passed shell targets were
not repeated unnecessarily; the remaining check recipe ran with explicit skips:

```sh
rtk proxy env FACTORY_AGENT_ROLE=reviewer bash -c 'make -o selftest -o budget-selftest -o loop-selftest -o adversarial-review-selftest -o go-runtime-check check > /private/tmp/factory-controller-check-remainder.log 2>&1; result=$?; tail -20 /private/tmp/factory-controller-check-remainder.log; exit "$result"'
```

That command exited 0. Selected shell and remaining-gate output across the runs:

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

`rtk proxy env FACTORY_AGENT_ROLE=reviewer make check-drift` exited 0 and produced
no tracked harness adapter changes. The citation gate skips with the repository's
empty citation prefix; the new ADR citations were checked against numbered source.

Linux-target static analysis:

```sh
rtk proxy env GOOS=linux GOARCH=amd64 /private/tmp/factory-quality-tools/golangci-lint run --config packs/go/.golangci.yml ./...
```

```text
0 issues.
```

The initial Linux lint identified one test driver subprocess requiring a scoped
fixture justification. That comment-only correction preceded the clean rerun.


## Ordinary contention qualification

A later full-suite run again failed the real contention case. Private diagnostic
instrumentation traced that failure to opening the initially absent budget.lock,
not the snapshot reader. The diagnostics were removed before final delivery.
An isolated local experiment (no ledger or history code) reproduced concurrent
os.Root.OpenFile O_CREATE returning ENOENT; exclusive creation followed only on
ErrExist by a non-creating open completed 10,000 iterations. Commands/output:

```text
rtk proxy go run /private/tmp/probe-lock-open.go
iteration=0 error=openat budget.lock: no such file or directory
rtk proxy go run /private/tmp/probe-lock-exclusive.go
10000 passed
```

The probe programs were temporary diagnostic artifacts, not shipped code or a
claim about the exact platform defect. The enduring regression tests exercise
the permitted file-acquisition sequence and real concurrent controllers.


Independent lock-acquisition tests ran before the correction:

```sh
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test ./internal/budget -ginkgo.focus='persistent lock creation' -ginkgo.no-color -ginkgo.succinct -count=1
```

Four cases ran: two passed and two failed. A separate vanished-lock case also
failed before implementation. After the exclusive-creation correction, the final
28 internal controller race cases and 23 instrumented outside-in cases passed
with the outputs recorded above. The lock-wait fixture allows two seconds for
preflight under build load while still asserting preflight occurred, execution
never started and no history was created.

The evaluator then repeated the ordinary compiled-driver contention case:

```sh
rtk proxy env FACTORY_AGENT_ROLE=spec-writer bash -c 'for attempt in {1..30}; do go test ./acceptance -ginkgo.focus="admits only one competing controller" -ginkgo.no-color -ginkgo.succinct -count=1 || exit "$?"; done > /private/tmp/factory-controller-contention-30.log 2>&1; result=$?; tail -32 /private/tmp/factory-controller-contention-30.log; exit "$result"'
```

All 30 invocations exited successfully (1.313s to 1.744s). Assertions still require
one completed execution, one ordinary blocked result and preserved accounting.
Independent final security review found no actionable lock-acquisition issue.


## Final full source qualification

After the lock-creation correction, the complete source gate exited 0:

```sh
rtk proxy env FACTORY_AGENT_ROLE=reviewer bash -c 'PATH="/private/tmp/factory-quality-tools:$PATH" make go-runtime-source-check > /private/tmp/factory-controller-source-qualified.log 2>&1; result=$?; tail -28 /private/tmp/factory-controller-source-qualified.log; exit "$result"'
```

This executes shell syntax checks, Ginkgo enforcement, go vet, the complete
race suite, CLI build, golangci-lint, gosec and govulncheck. Final security output:

```text
  Issues : 0
No vulnerabilities found.
```

Linux-target golangci-lint was also rerun after the final correction and returned
`0 issues.` Independent correctness and security review reported no remaining
actionable findings. Linux runtime execution and GitHub CI are separate checks.


## PR #97 review follow-up

The advisory review completed successfully on the original head. Its claimed
persisted launch-error usage and ordinary interruption regression were refuted:
Ledger.Finalize clears launch-error claims or refuses contradictory no-PID exit
evidence; the native executor records a PID before invoking the spawn callback
and ordinary confirmed cancellation returns the interrupted outcome with nil
error. An independent real-child test passed:

```sh
rtk proxy env FACTORY_AGENT_ROLE=reviewer go test ./acceptance -count=1 -v -ginkgo.focus='finalizes an interrupted native child after parent cancellation' -ginkgo.no-color
```

```text
1 Passed | 0 Failed
ok  github.com/anoop2811/software-factory-template/acceptance 2.232s
```

The controller nevertheless now skips parsing after its completion is downgraded
to launch_error. An independently authored inconsistent-collaborator case first
failed because parsing ran once; it does not demonstrate persisted corruption:

```sh
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test ./internal/budget -ginkgo.focus='Budget controller missing process identity' -ginkgo.no-color -ginkgo.succinct -count=1
```

RED: one case ran, zero passed, one failed (parsed=1, expected=0).
The correction also adds explicit boolean grouping and diagnostic assertions
before the fixture indexes the Go directive. No dependencies change.

Before adding the regression, enumeration confirmed the original 28 internal
controller cases; the review's suggested 25 count was incorrect:

```sh
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test ./internal/budget -ginkgo.focus='Budget controller' -ginkgo.dry-run -ginkgo.v -ginkgo.no-color -count=1 -v
```

This enumerated 28 of 33 cases; dry-run is not behavioral execution evidence.
The new regression raises the controller count to 29.


Final focused race qualification after the review correction:

```sh
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test -race ./internal/budget -ginkgo.focus='Budget controller' -ginkgo.no-color -ginkgo.succinct -count=1
rtk proxy env FACTORY_AGENT_ROLE=spec-writer FACTORY_CONTROLLER_TEST_RACE=1 go test -race ./acceptance -ginkgo.focus='G2 composed budget controller' -ginkgo.no-color -ginkgo.succinct -count=1
```

```text
ok  github.com/anoop2811/software-factory-template/internal/budget 6.937s
ok  github.com/anoop2811/software-factory-template/acceptance 15.624s
```

These cover 29 internal and 23 outside-in cases. The evaluator redirected output
to local logs. `rtk proxy env FACTORY_AGENT_ROLE=reviewer go vet ./internal/budget
./acceptance` exited 0 with no output; the Linux-target lint command above was
rerun and returned `0 issues.` The full source gate evidence earlier describes
the original PR head; this small follow-up was qualified with affected suites.
