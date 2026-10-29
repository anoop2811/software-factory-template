# Native Go initialization

Decision 84 and [ADR-0085](../adr/0085-go-native-init.md) define the native
prompting and publication contract. [ADR 0108](../adr/0108-default-go-cutover.md)
changes delivery so ordinary initialization selects a qualified Go installation.
From an installed Go factory:

```sh
./factory init /path/to/project --pack go,typescript
```

The installed release version is reauthenticated before initializing another
project. Use `--ref TAG` for an exact stable Go release, or `--go-candidate TAG`
for an explicitly selected published prerelease. The public installer keeps its
existing stable pin until the Go release is qualified and activated; unreleased
tags refuse rather than selecting another implementation.

The initializer collects project settings, waits for confirmation, installs the
selected packs and factory assets, synchronizes OpenCode, Claude Code and Codex,
and requires the installed gate selftests to pass. It does not invoke a model.
Dependency installation through npm remains an explicit external operation.
Synchronization and review-lane generation use their canonical Go implementations.
A nonzero attestation result means the
installation has not passed its checks; no automatic rollback is claimed.

For development, the source checkout's `./factory` builds the Go CLI. Explicit
`./factory init /path/to/project --local --pack go,typescript` packages committed
source, stages it with local trust and installs the qualified Go selection. This
requires the developer Go toolchain and reports its exact source identity; it is
not published authentication. It does not create a Bash scaffold as an intermediate
installation. Ordinary packaged execution needs neither Go nor Python.

The intended compatibility boundaries preserve user README content and unmanaged
Makefile/ignore rules. Unsafe source or destination paths and unrepresentable flat
configuration inputs must fail before installation writes. Reinitialization backups
are distinct from the migration recovery manifests; they do not establish authority
to restore or prune an installation.

## Historical native prompting/publication evidence

These results describe the earlier source-native initializer implementation.
They do not substitute for ADR 0108's new qualified delivery, cleanup and pilot
evidence, or for actual published release authentication.

The retained 85-case historical-source component family uses a guarded test-only
driver for the current initializer core and a frozen asset declaration. Its exact
legacy oracle, unsafe-path, terminal, publication and child-ownership assertions
remain in force. This is component evidence, not a claim that ordinary Go init
still copies the Bash scaffold. Real public default/local delivery and qualified
installation images have separate current acceptance cases.

Independent outside-in tests first observed three failures: the compiled command
still executed a poisoned legacy initializer. The corrected command does not
require that script. Additional regressions covered generated workflow syntax,
Git worktree redirection, literal JSON data, exported prompt values and temporary
file identity before their corrections.

The final targeted command used both a race-instrumented CLI and race-instrumented
acceptance process:

```sh
FACTORY_CLI_TEST_RACE=1 go test -race ./acceptance -count=1 -v \
  -ginkgo.focus='Native Go init (core|preservation|input and child ownership|blocked prompt cancellation|terminal input|interactive review choice|review secret representation|exported prompt compatibility)' \
  -ginkgo.no-color
```

Observed: `15 Passed | 0 Failed`, package `24.897s`, with no selected cases skipped.
The earlier 79-case instrumented CLI run passed before the final publication and
interactive-review changes; it is not evidence for a final 82-case instrumented run.
An independent final `go test -race ./acceptance -ginkgo.focus='Native Go init'`
passed in `192.450s`; that run instrumented the outer test process only.

```sh
go test -race ./internal/initcmd -count=1 -v \
  -ginkgo.focus='Native init prepared-file ownership' -ginkgo.no-color
```

Observed: `2 Passed | 0 Failed`, package `1.329s`. The security verifier additionally
ran the seven generated-workflow/Git-root compiled cases with CLI and outer race
instrumentation: `7 Passed | 0 Failed`, package `5.205s`.

`FACTORY_AGENT_ROLE=reviewer ./scripts/selftest/run.sh` returned
`selftest: 217 passed, 0 failed, 0 skipped`. Independent correctness and security
reviews found no remaining actionable findings. These results do not qualify
installed Go activation or migration backup retention.
