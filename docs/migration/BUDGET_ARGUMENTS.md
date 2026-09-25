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

The later ADR-0072 help qualification enumerates three malformed-input boundaries:
mixed short-help groups, ambiguity after help and a root terminator before a
command plus help. Go retains its documented refusals, while Python's responses
changed across releases. These cases have fixed Go assertions and separate
interpreter characterization rather than an unrestricted status-parity claim.

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

## Linux CI interpreter difference

OBSERVED 2026-09-25 UTC at d0e92b9: the Linux source job failed only the
`short mixed group` differential, with Go status 2 versus Python help/status 0.
The [Actions job](https://github.com/anoop2811/software-factory-template/actions/runs/36081983647/job/107905725836)
reported `1072 Passed | 1 Failed | 12 Skipped` and acceptance `168.280s`.
The local Python 3.12.0 had returned 2 for the same immutable factory source.

Executing official tagged argparse modules under one interpreter identified
v3.12.2 versus v3.12.3's different handling of the unknown suffix after `-h`.
The source snapshot freezes factory code, not the installed standard library.
Production follows ADR-0072's existing refusal contract. The evaluator separates
fixed Go refusals from characterization of the ambient Python behavior;
no success status is accepted from Go.

The subsequent [macOS job](https://github.com/anoop2811/software-factory-template/actions/runs/36081983647/job/107905726002)
failed six argument comparisons (`1061 Passed | 6 Failed | 18 Skipped`, acceptance
`320.777s`). Local Python 3.14.6 reproduced the additional help-precedence and
negative-number grammar differences. CI now selects maintained Python 3.12.14
on both platforms using pinned uv and asserts the exact interpreter version.
The actual managed 3.12.14 run initially passed 109 of 111 cases and failed only
the two backported help-precedence cases; these now join the explicitly
enumerated fixed-Go qualification. The negative-number comparisons remain strict.
RAN the final focused matrix with the actual managed Python 3.12.14 interpreter
and race-instrumented compiled candidate:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer FACTORY_BUDGET_COMMAND_TEST_RACE=1 bash -c 'export PATH=/private/tmp/factory-pr99-python-lsg4o68n/venv/bin:$PATH; go test -race ./acceptance -ginkgo.focus="G2 budget (argument|malformed short-help|maintained help-precedence|command)" -ginkgo.no-color -ginkgo.succinct -count=1 -v > /private/tmp/pr99-native31214-final-race.log 2>&1; tail -35 /private/tmp/pr99-native31214-final-race.log'
Ran 111 of 1087 Specs in 48.804 seconds
SUCCESS! -- 111 Passed | 0 Failed
ok github.com/anoop2811/software-factory-template/acceptance 50.396s
```

The 111 cases comprise 69 argument/help cases and 42 command/controller cases.
RAN `rtk proxy /private/tmp/factory-pr99-python-lsg4o68n/venv/bin/python3 -c 'import sys; print(sys.version); assert sys.version_info[:3] == (3,12,14)'`:
`3.12.14 (main, Sep 24 2026, 17:46:32) [Clang 22.1.3 ]`.
Targeted lint returned `0 issues.` RAN the repository-pinned workflow linter,
`rtk proxy go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 .github/workflows/go-runtime.yml`:
exit 0, no diagnostics. Full platform CI after this correction remains pending.

For local reproduction, provision a temporary managed Python 3.12.14 environment
with uv and prepend its `bin` directory to PATH before running the source gate.
This selects the oracle's standard library as well as the application snapshot.
The CI environment lives under `runner.temp`; it adds no project dependency or
installed-factory requirement. Setup/action versions and authoritative release
sources are recorded in ADR-0072.
