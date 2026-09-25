# Go manual loop controller qualification

[ADR-0075](../adr/0075-go-manual-loop-controller.md) governs milestone three
of the existing loop package: deterministic checks and fresh manual resume.
The private compiled candidate composes existing source identities, schema-one
checkpoints and process supervision. It does not activate installed commands
or add paid bounded loops. Public argument compatibility, delivery and legacy
cleanup remain pending.

## Evidence

RAN independent compiled-CLI core acceptance before production implementation:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test ./acceptance -ginkgo.focus='G2 manual loop core' -ginkgo.no-color -ginkgo.succinct -count=1 -v
Ran 3 of 1275 Specs in 1.644 seconds
FAIL! -- 0 Passed | 3 Failed | 0 Pending | 1272 Skipped
--- FAIL: TestAcceptance (1.66s)
FAIL github.com/anoop2811/software-factory-template/acceptance 2.082s
```

The immutable Python oracle successfully planned and ran passing/failing checks.
The candidate returned unsupported request before effects. Production followed
this RED. Focused and full source qualification are recorded below.

## Deadline regression

Independent review identified allowance calculation before checkpoint publication.
The evaluator injected a 1.1-second phase-save delay into a one-second loop:
execution was called once after the total allowance had expired. RAN before the
correction:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test ./internal/loop -ginkgo.focus='Manual loop total deadline' -ginkgo.no-color -ginkgo.succinct -count=1 -v
Ran 1 of 36 Specs in 1.252 seconds
FAIL! -- 0 Passed | 1 Failed | 0 Pending | 35 Skipped
FAIL github.com/anoop2811/software-factory-template/internal/loop 1.696s
```

The correction carries an absolute invocation deadline across preparation and
execution and recomputes the allowance after phase publication. Expiry during
fsync poisons the transaction; the regression requires no execution and the last
durable active recovery barrier, not an invented terminal save. RAN the corrected internal lifecycle cases with race detection:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test -race ./internal/loop -ginkgo.focus='Manual loop' -ginkgo.no-color -ginkgo.succinct -count=1 -v
--- PASS: TestLoopSnapshot (2.96s)
ok github.com/anoop2811/software-factory-template/internal/loop 4.324s
```

All 11 selected cases passed. Independent security review reran the same 11
cases with race detection (4.291s), finding no surviving issue.

## Validation-order regression

RAN the compiled marker-probe regression before correction:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test ./acceptance -ginkgo.focus='G2 manual loop validation order' -ginkgo.no-color -ginkgo.succinct -count=1 -v
FAIL github.com/anoop2811/software-factory-template/acceptance 2.355s
```

All three plan/run/resume cases failed: Python refused corrupt loop history
without invoking grep, while Go probed configuration first. The correction reads
history before configuration probes. Final qualification is tracked below.

The initial expanded 40-case run passed 38 and failed two because an active-budget
fixture retained a completed record's ended_at. That fixture is corrected to a
valid active record. The earlier checkpoint refusal fixture had the same latent
problem: malformed-state refusal did not establish active-budget refusal. Its
narrow correction is included without weakening the refusal assertion.

## Deliberate boundaries

Plans make no model calls or state writes. Runtime records do not prove native
agent enforcement. Signal handling uses observed child ownership rather than
blanket uncertainty; the precise private qualification difference is documented
in ADR-0075 and must be reconciled before installed activation.

## Signal-fixture synchronization

The first instrumented combined run passed 49 of 51 selected cases; two signal
fixtures could signal between PID-file rename and directory fsync. A visible
record was not proof the ownership callback had finished, so cancellation could
correctly poison publication instead of emitting a terminal record. The fixture
now waits for empty-stdin EOF before publishing its ready marker: the supervisor
closes stdin only after the ownership callback completes. This establishes the
intended after-admission interruption without a startup sleep. Exact interruption,
cleanup, state and output assertions remain in place.

## Final focused qualification

RAN the compiled CLI with race instrumentation for all manual cases and the
corrected checkpoint blocker fixtures:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer FACTORY_CLI_TEST_RACE=1 go test -race ./acceptance -ginkgo.focus='G2 manual loop|G2 loop checkpoint global resume blockers' -ginkgo.no-color -ginkgo.succinct -count=1 -v
--- PASS: TestAcceptance (36.07s)
ok github.com/anoop2811/software-factory-template/acceptance 37.557s
```

All 51 selected cases passed: 43 manual CLI cases and eight checkpoint blocker
cases. Other Ginkgo cases were excluded, not conditionally skipped within this
matrix. The CLI tests use real processes/filesystems and an immutable Python
oracle; harness names are metadata and no model was called. Eleven internal
race cases cover publication failures, no-retry barriers, ownership, lock lifetime,
captured environment and output deadlines. Correctness and security reviews
reported no remaining actionable findings after independent regression checks.

RAN supporting checks:

```text
rtk proxy make selftest
selftest: 217 passed, 0 failed, 0 skipped
rtk proxy env GOOS=linux GOARCH=amd64 /private/tmp/factory-quality-tools/golangci-lint run --config packs/go/.golangci.yml ./...
0 issues.
```

Linux-target lint is static analysis, not Linux runtime qualification. The full
repository source gate passed as recorded below. No dependency versions or go.sum changed.
Main-targeted GitHub CI and review do not run their normal gates on this stacked
feature base; local qualification remains distinct from release/live evidence.

The first full source-gate run compiled before that fixture-only correction and
failed in the same two signal fixtures (1298 passed, two failed, 15 skipped;
acceptance package 454.224s). Its production implementation was unchanged. The
full gate was rerun against the final frozen source; the earlier run remains
recorded as failed.

## Complete source gate

RAN with the qualified tools directory on PATH:

```text
PATH="/private/tmp/factory-quality-tools:$PATH" make go-runtime-source-check
ok github.com/anoop2811/software-factory-template/acceptance 453.523s
ok github.com/anoop2811/software-factory-template/internal/budget 7.917s
ok github.com/anoop2811/software-factory-template/internal/budgetcmd 6.079s
ok github.com/anoop2811/software-factory-template/internal/input 1.658s
ok github.com/anoop2811/software-factory-template/internal/loop 9.191s
ok github.com/anoop2811/software-factory-template/internal/native 2.513s
0 issues.
Issues : 0
No vulnerabilities found.
```

The command exited 0: shell syntax, Ginkgo dialect, vet, repository race tests,
build, lint, gosec and govulncheck passed. The full suite retains its existing
platform/fixture qualification boundaries; the focused 51-case matrix above had
no conditional skips. RAN diff-aware checks against the checkpoint PR base:

```text
rtk proxy scripts/hooks/diff-aware-check.sh 20391c2bf085cf1d5401ab921461d400201f44e8 HEAD
selftest: 217 passed, 0 failed, 0 skipped
diff-aware-check: all 1 dispatched check(s) passed
```
