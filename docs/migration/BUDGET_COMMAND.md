# Go budget command candidate

[ADR-0071](../adr/0071-go-budget-command-candidate.md) adds the private compiled
boundary `FACTORY_BRIDGE_PROTOCOL=1 factory budget controller ...`.
It composes the existing ledger, runner and usage/native components using Cobra.
It does not change the public installed `factory budget` dispatcher.

## Compatibility scope

The source candidate covers exact plan/run/report commands and their supported
long flags, including equals forms, read-only reporting, text/JSON metadata,
pre-execution plan output and post-finalization text answers. Report does not
require valid execution configuration. JSON is newline-delimited metadata only.

Python argparse help/error prose, abbreviated long flags and short-option edge
cases remain open compatibility obligations. The private command refuses them
explicitly. Shell YAML/model selection, loop integration, artifact activation,
gitignored recovery backups and old-script retirement remain separate gates.
No dependency is added and no paid model calls are used for validation.

A run emits its initial blocked plan or its locked current plan exactly once.
If another process consumes the last slot during preflight, the locked blocked
plan is still emitted. Output errors before reservation prevent execution;
errors printing finalized metadata never undo accounting or retry the run.

## Evidence

The first four plan/report comparisons were written before implementation, but
used an incorrect extra `__bridge` token. Their initial failures are fixture
setup evidence, not qualified RED for the corrected protocol. The existing
protocol is selected by FACTORY_BRIDGE_PROTOCOL and takes `budget controller`
directly. That invocation was corrected after implementation had begun.

The independent evaluator subsequently compares the corrected cases against an
isolated binary built from immutable parent ad1ca7f, separately from the current
candidate. This establishes the missing-boundary regression, but must not be
represented as strict pre-implementation RED for the corrected route. Additional
behavioral failures observed before their corrections are recorded separately.

Source implementation does not establish installed compatibility or release
readiness. Final qualification is recorded below only after execution.


### Focused source qualification (2026-09-23)

RAN with FACTORY_AGENT_ROLE=spec-writer:

- `FACTORY_BUDGET_COMMAND_TEST_RACE=1 FACTORY_CONTROLLER_TEST_RACE=1 go test -race ./acceptance -ginkgo.focus="G2 budget command" -ginkgo.no-color -ginkgo.succinct -count=1`: `ok github.com/anoop2811/software-factory-template/acceptance 18.421s` (44 cases). The compiled candidate and fake harness are race-instrumented explicitly.
- `go test -race ./internal/budget ./internal/budgetcmd -ginkgo.focus="Budget command" -ginkgo.no-color -ginkgo.succinct -count=1`: `ok github.com/anoop2811/software-factory-template/internal/budget 1.816s` and `ok github.com/anoop2811/software-factory-template/internal/budgetcmd 3.951s` (17 and 10 cases).

The corrected four core cases failed against the isolated parent binary (actual
status 2, expected 0). Supplemental RED before correction covered explicit
boolean values/help, hidden completion output and real-child cancellation
returning status 1 instead of 130. The final focused runs above passed.

Final output uses a fresh five-second context after durable finalization; this
does not impose an interruptible deadline on arbitrary blocking writer syscalls.

RAN `make selftest budget-selftest loop-selftest adversarial-review-selftest`:

```text
selftest: 217 passed, 0 failed, 0 skipped
budget: 32 passed, 0 failed
loop: 40 passed, 0 failed
adversarial-review: 53 passed, 0 failed
```

RAN `scripts/hooks/diff-aware-check.sh ad1ca7f HEAD`:
`diff-aware-check: all 1 dispatched check(s) passed` (217 self-tests).
Independent correctness and security reviews found no remaining actionable
findings after the command/cancellation corrections.

RAN `PATH="/private/tmp/factory-quality-tools:$PATH" make go-runtime-source-check`
(exit 0): all Go packages passed `go test -race -count=1 ./...`; vet and build
passed. Lint output: `0 issues.` Gosec output: `Issues : 0`. Govulncheck output:
`No vulnerabilities found.` This is local source qualification; Linux/macOS CI
for the new PR remains separate evidence.
