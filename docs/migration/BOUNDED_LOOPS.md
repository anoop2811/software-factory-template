# Go bounded loop command qualification

[ADR-0076](../adr/0076-go-bounded-loop-controller.md) governs milestone four
of the existing loop package: bounded implementation/check/review/repair through
the shared budget controller, and the private Cobra command candidate.
Installed dispatch, safe upgrade activation and legacy retirement remain pending.

## Independent RED

RAN the evaluator's compiled-CLI core acceptance before production implementation:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test ./acceptance -ginkgo.focus='G2 bounded loop command core' -ginkgo.no-color -ginkgo.succinct -count=1 -v
Ran 3 of 1318 Specs in 1.397 seconds
FAIL! -- 0 Passed | 3 Failed | 0 Pending | 1315 Skipped
--- FAIL: TestAcceptance (1.41s)
FAIL github.com/anoop2811/software-factory-template/acceptance 1.823s
```

The immutable Python oracle returned manual plan/status success and bounded
disabled-plan refusal. The candidate returned unsupported request. Production
implementation followed this observed RED.

## Blocked reviewer diagnostic regression

The first expanded differential run observed a mismatch when the implementer used
the last shared budget attempt: both controllers prevented the reviewer call, but
the candidate returned a generic invocation-failure reason. Python distinguishes
admission/native-readiness refusal from an admitted execution failure. RAN:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test ./acceptance -ginkgo.focus='G2 bounded loop' -ginkgo.no-color -ginkgo.succinct -count=1 -v
Ran 48 of 1363 Specs in 35.294 seconds
FAIL! -- 47 Passed | 1 Failed | 0 Pending | 1315 Skipped
--- FAIL: TestAcceptance (35.31s)
FAIL github.com/anoop2811/software-factory-template/acceptance 35.812s
```

The correction preserves that distinction using the returned admission record.
Final focused qualification of the correction is recorded below.

## Prompt refusal before effects

The evaluator then observed that the candidate created `.factory` before refusing
a missing admitted prompt, unlike the immutable oracle. RAN the independent
regression before correction:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test ./acceptance -ginkgo.focus='G2 bounded loop prompt admission ordering' -ginkgo.no-color -ginkgo.succinct -count=1 -v
Ran 1 of 1367 Specs in 1.052 seconds
FAIL! -- 0 Passed | 1 Failed | 0 Pending | 1366 Skipped
FAIL github.com/anoop2811/software-factory-template/acceptance 1.391s
```

Prompt validation now precedes lock acquisition. The locked freshness check still
rereads and compares the prompt before invoking a native client. Independent
security review found no weakened freshness boundary in this correction.

## Final focused qualification

RAN the corrected bounded command with its actual compiled binary race-instrumented:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer FACTORY_CLI_TEST_RACE=1 go test -race ./acceptance -ginkgo.focus='G2 bounded loop' -ginkgo.no-color -ginkgo.succinct -count=1 -v
ok github.com/anoop2811/software-factory-template/acceptance 43.187s
```

All 52 selected cases passed. Before the narrow prompt-order correction, the
combined bounded/manual/budget-grammar regression run also passed all 161 cases:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer FACTORY_CLI_TEST_RACE=1 FACTORY_BUDGET_COMMAND_TEST_RACE=1 go test -race ./acceptance -ginkgo.focus='G2 bounded loop|G2 manual loop|G2 budget argument' -ginkgo.no-color -ginkgo.succinct -count=1 -v
ok github.com/anoop2811/software-factory-template/acceptance 89.518s
```

That run comprised 51 bounded, 43 manual and 67 budget argument cases. The new
ordering regression makes the final bounded count 52. RAN all 32 bounded internal
lifecycle and strict-verdict cases with race detection:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test -race ./internal/loop -ginkgo.focus='Bounded loop' -ginkgo.no-color -ginkgo.succinct -count=1 -v
ok github.com/anoop2811/software-factory-template/internal/loop 2.871s
```

Independent correctness and security reviews found no surviving actionable
findings. The security reviewer independently ran the 32 internal race cases and
17 compiled safety/signal/deadline/prompt/repair cases successfully.

## Pinned interpreter follow-up

PR #99's platform CI exposed host-argparse drift. Its correction pins the oracle
to maintained Python 3.12.14 and enumerates three malformed-help qualifications
in ADR-0072. The loop grammar inherits those fixed Go refusal requirements;
its two affected vectors now characterize Python separately without weakening
Go status/channel or no-effects assertions. Production code is unchanged.

RAN the final combined acceptance against the actual managed interpreter:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer FACTORY_CLI_TEST_RACE=1 FACTORY_BUDGET_COMMAND_TEST_RACE=1 bash -c 'export PATH=/private/tmp/factory-pr99-python-lsg4o68n/venv/bin:$PATH; go test -race ./acceptance -ginkgo.focus="G2 bounded loop|G2 manual loop|G2 budget (argument|malformed short-help|maintained help-precedence)" -ginkgo.no-color -ginkgo.succinct -count=1 -v'
ok github.com/anoop2811/software-factory-template/acceptance 92.312s
```

All 164 selected cases passed: 52 bounded, 43 manual and 69 budget grammar.
Acceptance lint returned `0 issues.` The complete source gate is rerunning with
the pinned interpreter; the earlier successful source gate below used Python 3.12.0.

The subsequent PR #99 review identified an explicit short-help value bug in the
shared grammar: `-h=h` and `-h=hh` lost `=` and succeeded as repeated help. The
evaluator added loop root/leaf regressions before correcting the shared helper:
`G2 bounded loop` explicit-value focus ran six cases, four failed and two passed
(`acceptance 3.231s`). Empty `-h=` already failed correctly. The correction
preserves the raw suffix and retains valid repeated-help groups. RAN the final
combined instrumented matrix with that correction:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer FACTORY_CLI_TEST_RACE=1 FACTORY_BUDGET_COMMAND_TEST_RACE=1 bash -c 'export PATH=/private/tmp/factory-pr99-python-lsg4o68n/venv/bin:$PATH; go test -race ./acceptance -ginkgo.focus="G2 bounded loop|G2 manual loop|G2 budget (argument|malformed short-help|maintained help-precedence)" -ginkgo.no-color -ginkgo.succinct -count=1 -v'
ok github.com/anoop2811/software-factory-template/acceptance 104.567s
```

All 176 cases passed: 58 bounded, 43 manual and 75 budget grammar. An intermediate
pinned-interpreter source gate also exited 0 (`acceptance 552.834s`, lint 0,
gosec 0, no vulnerabilities), but overlapped this later correction and is not
claimed as final-snapshot evidence. The final frozen production snapshot passed
the complete pinned-interpreter gate recorded below.

## Complete source gate

RAN the complete gate on the corrected bounded-loop snapshot:

```text
rtk proxy bash -c 'export PATH="/private/tmp/factory-quality-tools:$PATH"; export FACTORY_AGENT_ROLE=reviewer; make go-runtime-source-check > /private/tmp/factory-bounded-source-check.log 2>&1'
ok github.com/anoop2811/software-factory-template/acceptance 511.445s
ok github.com/anoop2811/software-factory-template/internal/budget 8.048s
ok github.com/anoop2811/software-factory-template/internal/budgetcmd 6.050s
ok github.com/anoop2811/software-factory-template/internal/input 2.066s
ok github.com/anoop2811/software-factory-template/internal/loop 11.090s
ok github.com/anoop2811/software-factory-template/internal/native 2.535s
0 issues.
Issues : 0
No vulnerabilities found.
```

The command exited 0 after format/dialect checks, vet, all Go race tests, build,
lint, gosec and govulncheck. RAN Linux-target static lint:

```text
rtk proxy env GOOS=linux GOARCH=amd64 /private/tmp/factory-quality-tools/golangci-lint run --config packs/go/.golangci.yml ./...
0 issues.
```

RAN `rtk proxy bash -c './scripts/selftest/run.sh > /private/tmp/factory-bounded-shell-selftest.log 2>&1'`:
`selftest: 217 passed, 0 failed, 0 skipped`. Structural evaluations passed for
Codex, Claude and OpenCode. Linux execution and GitHub CI for this new bounded
slice remain pending. No real model calls or live native-client enforcement
qualification is claimed. Fake native executables exercise the controller boundary.

## Final pinned-interpreter source gate

RAN the complete gate after the explicit-help correction, with production frozen:

```text
rtk proxy bash -c 'export PATH="/private/tmp/factory-pr99-python-lsg4o68n/venv/bin:/private/tmp/factory-quality-tools:$PATH"; export FACTORY_AGENT_ROLE=reviewer; make go-runtime-source-check > /private/tmp/factory-bounded-final-source-check.log 2>&1'
ok github.com/anoop2811/software-factory-template/acceptance 542.276s
ok github.com/anoop2811/software-factory-template/internal/budget 7.477s
ok github.com/anoop2811/software-factory-template/internal/budgetcmd 5.695s
ok github.com/anoop2811/software-factory-template/internal/input 2.583s
ok github.com/anoop2811/software-factory-template/internal/loop 11.061s
ok github.com/anoop2811/software-factory-template/internal/native 2.879s
0 issues.
Issues : 0
No vulnerabilities found.
```

The command exited 0, covering format/dialect checks, vet, the complete Go race
suite, build, lint, gosec and govulncheck under Python 3.12.14. This is local
source evidence; the bounded change's GitHub platform CI remains pending.

## Integrated device-input correction

The PR #101 terminal-input finding was corrected in the shared input adapter and
carried into this candidate. Its RED, eight input race cases and compiled PTY
refusal evidence are recorded in LOOP_CHECKPOINTS.md. RAN the complete source
gate again with the corrected adapter:

```text
rtk proxy bash -c 'export PATH="/private/tmp/factory-pr99-python-lsg4o68n/venv/bin:/private/tmp/factory-quality-tools:$PATH"; export FACTORY_AGENT_ROLE=reviewer; make go-runtime-source-check > /private/tmp/factory-bounded-device-final-source-check.log 2>&1'
ok github.com/anoop2811/software-factory-template/acceptance 500.382s
ok github.com/anoop2811/software-factory-template/internal/budget 7.664s
ok github.com/anoop2811/software-factory-template/internal/budgetcmd 6.517s
ok github.com/anoop2811/software-factory-template/internal/input 2.507s
ok github.com/anoop2811/software-factory-template/internal/loop 10.681s
ok github.com/anoop2811/software-factory-template/internal/native 3.018s
0 issues.
Issues : 0
No vulnerabilities found.
```

The command exited 0. No further production changes followed this run.

## Prompt cancellation review follow-up

PR #103 review exposed cancellation returned by the bounded prompt reader being
replaced with the generic validation error. ADR-0076:154 clarifies that context
identity must survive this helper, without promising different caller handoff text.
Independent deterministic mid-read cancellation and deadline cases observed RED:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test ./internal/loop -ginkgo.focus="Bounded loop prompt cancellation" -ginkgo.no-color -count=1 -v
0 Passed | 2 Failed | 78 Skipped
FAIL github.com/anoop2811/software-factory-template/internal/loop 0.385s
```

After the three-line context check, RAN the bounded internal race suite:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test -race ./internal/loop -ginkgo.focus="Bounded loop" -ginkgo.no-color -count=1 -v
34 Passed | 0 Failed | 46 Skipped
ok github.com/anoop2811/software-factory-template/internal/loop 2.967s
```

Targeted loop lint returned `0 issues.` The new cases require no partial prompt
and preserved cancellation/deadline identity. Fresh GitHub CI is required for
this correction; the earlier full-source results remain historical evidence.

RAN the compiled CLI bounded-loop matrix with race instrumentation and the pinned
Python oracle interpreter after this correction:

```text
rtk proxy env FACTORY_AGENT_ROLE=reviewer FACTORY_CLI_TEST_RACE=1 bash -c 'export PATH=/private/tmp/factory-pr99-python-lsg4o68n/venv/bin:$PATH; go test -race ./acceptance -ginkgo.focus="G2 bounded loop" -ginkgo.no-color -ginkgo.succinct -count=1 -v'
58/1381 specs SUCCESS!
PASS
ok github.com/anoop2811/software-factory-template/acceptance 45.470s
```

Independent correction review found no remaining actionable issue.
