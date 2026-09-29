# Native Go health diagnostics

Source implementation is qualified locally under Decision 85 and
[ADR-0086](../adr/0086-go-native-doctor.md). Installed runtime activation remains
separate. Build the developer command in the factory source checkout:

```sh
go build -o factory-go ./cmd/factory
./factory-go doctor
```

The source command reports gate configuration, hook integrity, adapter drift and
break/fix proof results. An inert gate is a configuration choice, while a missing
required hook or a failed proof makes the command fail. Warnings remain visible
without turning deliberate opt-outs into errors. Arguments retain the legacy
doctor behavior: they are ignored, including --help.

The adapter comparison runs the retained generators in a private scratch
copy instead of replacing live adapter files. It reports unsafe or incomplete
inspection rather than claiming agreement. Its CODEOWNERS check reports literal
path references, not remote branch-protection enforcement or full rule semantics.

Shared Claude/Codex adapter generators, review-lane status helpers and the existing
break/fix proof remain subprocess boundaries. Doctor never requests a model review
or installs dependencies. Custom repository tools remain trusted code; this is not
a sandbox for arbitrary custom selftests. Signals cancel owned child work, and
bounded reads, output capture and deadlines prevent indefinite diagnostic work.

No configuration or Git setting is changed by native diagnostic orchestration.
Running this source command does not activate Go in an existing installation,
retire scripts or create migration recovery state.

## Qualification

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
remaining actionable findings. Repository-wide gates and exact-head CI remain
pending.
This evidence does not qualify installed activation or script retirement.
