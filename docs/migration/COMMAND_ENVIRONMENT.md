# Go command environment qualification

[ADR-0077](../adr/0077-go-command-environment.md) governs shared configuration
and model routing for the private compiled budget and loop commands. It is the
first of three milestones in the remaining G2 integration/retirement package.
The 30-package progress denominator remains unchanged.

## Compatibility boundary

The immutable wrapper oracle is commit
`c8f8d34edbc14df5655fcbf0aab7eea46ced0295`. Candidate requests are
`FACTORY_BRIDGE_PROTOCOL=1 factory budget configured ARGS` and
`FACTORY_BRIDGE_PROTOCOL=1 factory loop configured ARGS`.

Configuration preparation precedes command parsing, including help. Derived
budget and loop settings replace incoming values; exported model settings retain
caller-presence precedence, including explicit empty values. The budget wrapper's
literal model-selection scan remains separate from the command parser, including
its behavior for abbreviated options. Shared Go configuration and role helpers
remain the source of these values.

Configuration and working-directory state are assumed stable during preparation.
Configuration I/O failures have a private qualification: sanitized status 2 with
no command effects. Git discovery is bounded to five seconds and 1 MiB combined
output. PATH must be supplied; explicit empty PATH is supported, while absent
PATH is refused because Bash synthesizes build-dependent defaults. This exception
remains a public-activation compatibility obligation. No claim of byte-identical
legacy shell diagnostics or unlimited discovery follows.
Local qualification uses fake native clients and makes no paid model calls.

## Independent RED

RAN the independent compiled boundary before implementation with the maintained
Python oracle interpreter on PATH:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer bash -c 'gofmt -w acceptance/command_environment_test.go; export PATH=/private/tmp/factory-pr99-python-lsg4o68n/venv/bin:$PATH; go test ./acceptance -ginkgo.focus="G2 configured command core" -ginkgo.no-color -count=1 -v > /private/tmp/factory-command-environment-core-red.log 2>&1; result=$?; tail -15 /private/tmp/factory-command-environment-core-red.log; exit "$result"'
Ran 3 of 1384 Specs in 2.625 seconds
FAIL! -- 0 Passed | 3 Failed | 0 Pending | 1381 Skipped
FAIL github.com/anoop2811/software-factory-template/acceptance 2.962s
```

The immutable wrappers produced the expected budget plan/report and manual loop
plan. The candidate refused all three unsupported configured routes. Production
implementation followed this observed failure.

## Git descendant regression

A failing Git leader can leave a child holding an inherited output pipe. The
initial candidate treated the resulting nonzero exit as an ordinary root fallback.
The independent regression uses a child PID handshake before the leader exits:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test ./acceptance -ginkgo.focus="nonzero Git leader" -ginkgo.no-color -count=1 -v
FAIL! -- 0 Passed | 1 Failed | 0 Pending | 1452 Skipped
FAIL github.com/anoop2811/software-factory-template/acceptance 2.057s
```

## Absent PATH qualification regression

The initial candidate silently substituted a search path when PATH was absent.
ADR-0077 now explicitly requires a supplied PATH at this private boundary. RAN
before the correction:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test ./acceptance -ginkgo.focus="absent PATH qualification" -ginkgo.no-color -count=1 -v
FAIL! -- 0 Passed | 1 Failed | 0 Pending | 1453 Skipped
FAIL github.com/anoop2811/software-factory-template/acceptance 0.812s
```

## Focused qualification

RAN the actual compiled candidate with race instrumentation and the maintained
Python interpreter, covering all 76 new cases:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer FACTORY_CLI_TEST_RACE=1 bash -c 'export PATH=/private/tmp/factory-pr99-python-lsg4o68n/venv/bin:$PATH; go test -race ./acceptance -ginkgo.focus="G2 configured command" -ginkgo.no-color -count=1 -v > /private/tmp/factory-command-environment-final-race.log 2>&1; result=$?; tail -18 /private/tmp/factory-command-environment-final-race.log; exit "$result"'
Ran 76 of 1457 Specs in 64.009 seconds
SUCCESS! -- 76 Passed | 0 Failed | 0 Pending | 1381 Skipped
ok github.com/anoop2811/software-factory-template/acceptance 65.639s
```

Coverage includes 30 harness/role/profile combinations, caller/YAML/legacy
precedence, derived setting overwrite, root/config paths, safe argument handling,
manual and bounded fake-client execution for all three harnesses, environment
isolation and probe failure boundaries. No live client or paid invocation is
claimed. After a context-parameter-order-only helper cleanup for lint, its two
composer isolation cases also passed with race detection (`clones supplied values`,
acceptance 3.408s).

## Full-suite allowance

The initial `make go-runtime-source-check` run completed 1,435 passing acceptance
cases before Go's implicit package timeout: `panic: test timed out after 10m0s`,
`FAIL github.com/anoop2811/software-factory-template/acceptance 600.321s`. It
reported no assertion failure. A progress report showed the suite advancing
through existing CLI cases; the timeout occurred during an existing loop probe.
This run is a failure, not full-suite qualification.

ADR-0077 now gives this gate a configurable `GO_RUNTIME_TEST_TIMEOUT=15m` default.
Per-operation deadlines and the CI job limit remain unchanged. The corrected
full gate must pass; its completion evidence is recorded in the PR checks and
validation results, separately from the focused evidence above.

Independent correctness and security review found no remaining actionable issue.
The security reviewer separately ran the actual descendant ownership regression:

```text
rtk proxy env FACTORY_AGENT_ROLE=reviewer go test ./acceptance -count=1 -v -ginkgo.focus='G2 configured command probe ownership' -ginkgo.no-color
Ran 1 of 1457 Specs in 1.658 seconds
SUCCESS! -- 1 Passed | 0 Failed | 0 Pending | 1456 Skipped
```

Installed dispatch, upgrade recovery, local gitignored backups and removal of
legacy active files remain separate work. This candidate does not switch existing
users to the Go command or remove their scripts.
