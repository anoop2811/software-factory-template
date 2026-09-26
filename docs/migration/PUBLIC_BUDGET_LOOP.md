# Public Go budget and loop qualification

[ADR-0078](../adr/0078-go-public-budget-loop.md) governs public budget and loop
routing in the developer-built Go binary. It is milestone two of the existing G2
integration/retirement package; it does not change the 30-package denominator.

## Boundary

The source-built binary runs `factory budget ARGS` and `factory loop ARGS`
through the same configured runner as the private candidate routes. Other public
commands retain their script exec boundary. The tracked shell dispatcher, direct
scripts, adapters and installation remain unchanged.

The sole top-level help prose qualification is the compiled footer:
`Budget and loop use the Go runtime. Other commands use auditable scripts.`
Configuration, model selection and controller qualifications remain recorded in
COMMAND_ENVIRONMENT.md, BUDGET_ARGUMENTS.md and BOUNDED_LOOPS.md. In particular,
an absent PATH remains an explicit source-candidate refusal and an unresolved
installed-activation obligation. No fallback script runs after a native refusal
or failure.

## Independent RED

RAN the independent actual-public-command matrix before production changes:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer bash -c 'gofmt -w acceptance/public_commands_test.go; export PATH=/private/tmp/factory-pr99-python-lsg4o68n/venv/bin:$PATH; go test ./acceptance -ginkgo.focus="G2 public native command core" -ginkgo.no-color -count=1 -v > /private/tmp/factory-public-command-core-red.log 2>&1; result=$?; tail -25 /private/tmp/factory-public-command-core-red.log; exit "$result"'
Ran 3 of 1460 Specs in 2.802 seconds
FAIL! -- 0 Passed | 3 Failed | 0 Pending | 1457 Skipped
FAIL github.com/anoop2811/software-factory-template/acceptance 3.361s
```

The immutable wrappers returned the expected budget plan/report and manual loop
plan. The public candidate tried to delegate to missing colocated scripts. The
new Go routing followed this observed RED.

## Focused qualification

RAN the public commands, retained dispatcher boundary and private configured
routes with the actual compiled CLI race-instrumented:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer FACTORY_CLI_TEST_RACE=1 bash -c 'export PATH=/private/tmp/factory-pr99-python-lsg4o68n/venv/bin:$PATH; go test -race ./acceptance -ginkgo.focus="G2 public native command|developer-built Cobra command boundary|G2 configured command" -ginkgo.no-color -count=1 -v > /private/tmp/factory-public-command-final-race.log 2>&1; result=$?; tail -14 /private/tmp/factory-public-command-final-race.log; exit "$result"'
Ran 145 of 1481 Specs in 122.503 seconds
SUCCESS! -- 145 Passed | 0 Failed | 0 Pending | 1336 Skipped
ok github.com/anoop2811/software-factory-template/acceptance 124.221s
```

These are 28 public cases, 41 retained/updated dispatcher cases and 76 private
configured-command cases. Native failures preserve the adapter's status while
retaining the raw native exit in the ledger; they do not blindly return the child
exit code. Four signal cases use observed child readiness and test interruption,
no second invocation, no shell fallback and child exit.

Independent correctness and security reviews found no actionable issue. The
security reviewer separately ran all four signal cases with race instrumentation:

```text
rtk proxy env FACTORY_AGENT_ROLE=reviewer FACTORY_CLI_TEST_RACE=1 go test -race ./acceptance -count=1 -v -ginkgo.focus='G2 public native command failures.*cancels its observed native child' -ginkgo.no-color
4 Passed | 0 Failed
```

Targeted lint returned `0 issues.` Local clients are fake: no paid model calls or
live harness enforcement are claimed. Complete source-gate and platform CI
completion are recorded in the corresponding PR validation and check results.

Installed activation, mixed-runtime upgrade exclusion, recovery copies and legacy
retirement remain separate work. Selecting a local binary does not authenticate
it as an official release or authorize deletion of predecessor files.
