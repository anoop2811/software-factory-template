# Go budget argument compatibility

[ADR-0072](../adr/0072-go-budget-argument-compatibility.md) extends the private
Go budget command with the legacy help and argument grammar. It supersedes the
initial help/prefix refusal documented in [BUDGET_COMMAND.md](BUDGET_COMMAND.md).
The installed Bash/Python route, shell model selection and upgrade behavior
remain unchanged.

## Comparison contract

The oracle is scripts/lib/budget.py at immutable commit
4cc771e894e11d5024032106aeb0ef788997bd29. The independent evaluator compares
statuses and channels, help command/option discoverability, usage/error presence,
no-effects behavior and typed successful output. Human diagnostic/help wording,
wrapping, program path, color and Python-version-specific section headings are
normalized, as enumerated before tests in ADR-0072. Raw operand values are not
repeated in new diagnostics. These normalizations do not permit changes to
machine fields, argument arity, flag scope or successful command behavior.

This is a source interface qualification. It does not establish shell routing,
installed activation, cleanup, release responsiveness or complete G2 closure.

## Evidence

Before production edits, seven compiled-CLI cases failed for missing root/leaf
help, unique prefixes and usage-on-error. Four help-output cases also failed
before implementation. Later, plan/run required-option discoverability failed
before the shared metadata correction (report already passed).

RAN 2026-09-23 with FACTORY_AGENT_ROLE=spec-writer:

- `FACTORY_BUDGET_COMMAND_TEST_RACE=1 FACTORY_CONTROLLER_TEST_RACE=1 go test -race ./acceptance -ginkgo.focus='G2 budget (argument|command)' -ginkgo.no-color -ginkgo.succinct -count=1 -v`: `109 Passed | 0 Failed` and `ok github.com/anoop2811/software-factory-template/acceptance 42.262s`. Both the actual compiled candidate and fake native CLI binaries are instrumented.
- `go test -race ./internal/budgetcmd ./internal/budget -ginkgo.focus='Budget (argument|command)' -ginkgo.no-color -ginkgo.succinct -count=1 -v`: command `14 Passed | 0 Failed` (`3.559s`); budget `17 Passed | 0 Failed` (`2.032s`).

The 109 outside-in cases comprise 67 argument/help cases and 42 existing
command/controller cases. Former help/prefix refusal assertions were replaced
with stronger successful parity checks; invalid input still has no effects.
RAN `PATH="/private/tmp/factory-quality-tools:$PATH" make go-runtime-source-check`
(exit 0): vet, all-package race tests and build passed. Lint reported `0 issues.`;
gosec reported `Issues : 0`; govulncheck reported `No vulnerabilities found.`

RAN `scripts/hooks/diff-aware-check.sh a5eaa07 HEAD`:

```text
selftest: 217 passed, 0 failed, 0 skipped
diff-aware-check: all 1 dispatched check(s) passed
```

Independent correctness and security reviews found no remaining actionable
findings. The source budget-controller interface matrix is complete at this
boundary. Full main-target Linux/macOS CI, installed shell integration and
release qualification remain separate evidence; stacked-branch CI is not a
substitute for those checks.
