# Native Go health diagnostics

The source launcher, direct `scripts/factory-doctor.sh` entrypoint and
`make doctor` use the native Go diagnostic owner. From a factory source checkout:

```sh
./factory doctor
```

Source use requires Go; the launcher builds a private developer executable on
each invocation. An installed factory uses its packaged runtime without requiring
a compiler. The direct
script selects the runtime beside its physical source owner, while diagnostics
inspect the caller's Git root (or working directory outside Git). A relative
`FACTORY_CONFIG` is resolved against that selected project. See
[ADR 0129](../adr/0129-go-source-doctor-entrypoint.md).

The command reports gate configuration, hook integrity, adapter drift and
break/fix proof results. An inert gate is a configuration choice, while a missing
required hook or a failed proof makes the command fail. Warnings remain visible
without turning deliberate opt-outs into errors. Arguments retain the legacy
doctor behavior: they are ignored, including --help.

The adapter comparison uses native synchronization in a private scratch copy
instead of replacing live adapter files. It reports unsafe or incomplete
inspection rather than claiming agreement. Its CODEOWNERS check reports literal
path references, not remote branch-protection enforcement or full rule semantics.

Synchronization and review-lane status use their shared Go owners. The caller's
`scripts/selftest/run.sh` remains a supervised subprocess boundary for the native
fourteen-gate break/fix proof. The factory's separate development regression
runner is not selected by doctor. Doctor never requests a model review or installs dependencies.
Custom repository tools remain trusted code; this is not
a sandbox for arbitrary custom selftests. Signals cancel owned child work, and
bounded reads, output capture and deadlines prevent indefinite diagnostic work.

No configuration or Git setting is changed by native diagnostic orchestration.
Running this source command does not activate Go in an existing installation,
retire scripts or create migration recovery state.

## Historical qualification of Decision 85

The following observations record the original native diagnostic implementation
under [ADR 0086](../adr/0086-go-native-doctor.md), before the later source-entrypoint
activation. They describe that revision, not a new run or current CI status.

Independent compiled-command core tests observed three failures against the
legacy route before implementation: the missing doctor script produced status 2
instead of the required healthy/inert status 0 or missing-config status 1.
The first implemented report matrix observed 36 passed and one failed case: proof
stdout and stderr were concatenated in the wrong order. The correction reuses the
shared supervisor's merged stream. Additional regressions covered shell word
boundaries and removal of scratch trees containing read-only directories.

Final focused qualification instrumented both the actual CLI and the test runner:

```sh
FACTORY_CLI_TEST_RACE=1 go test -race ./acceptance -run TestAcceptance \
  -ginkgo.focus='Native Go doctor' -ginkgo.no-color -ginkgo.succinct -count=1
```

Observed: `ok .../acceptance 130.941s`. This includes real retained synchronization
and break/fix proof, with network/model clients replaced by offline fixtures, plus
report parity, configuration paths, bounded inputs, adapter preservation, progress
and signal cancellation. The real retained proof reported
`selftest: 217 passed, 0 failed, 0 skipped` during fixture qualification.
Two later guarded scratch-replacement cases ran with the same CLI and test-runner
race instrumentation using `-ginkgo.focus='Native Go doctor.*guarded scratch'`:
`ok .../acceptance 4.762s`. These are post-implementation GREEN checks, not a
claimed pre-correction failure. Independent correctness/security review has no
remaining actionable findings at that stage; repository-wide gates and exact-head
CI were still pending. This historical evidence alone does not qualify installed
activation or script retirement.
