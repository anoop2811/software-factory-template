# Native Go initialization

Source initialization and explicit local-image initialization have different
delivery behavior. Ordinary source initialization keeps the script installation;
the explicit local selector below installs its selected Go build.

Decision 84 and [ADR-0085](../adr/0085-go-native-init.md) define the native
initializer contract. From a factory source checkout, build alongside the
tracked shell entry point:

```sh
go build -o factory-go ./cmd/factory
./factory-go init /path/to/project --pack go,typescript
```

Keep the tracked `factory` file intact. The source-built command needs the
checkout's template assets beside it. A detached binary without those assets is
not an installer. This command initializes the project selected by its target
argument; it is not the read-only upgrade preview.

The initializer collects project settings, waits for confirmation, installs the
selected packs and factory assets, synchronizes OpenCode, Claude Code and Codex,
and requires the installed gate selftests to pass. It does not invoke a model.
Dependency installation through npm and shared synchronization/review-lane scripts
remain explicit external operations. A nonzero attestation result means the
installation has not passed its checks; no automatic rollback is claimed.

Native source initialization does not activate Go in the destination. The installed
entry point remains the template's existing shell dispatcher until the separate
runtime cutover is qualified. Existing installation upgrades, migration recovery
creation, rollback and retention remain separate work.

## Initialize from committed local source

The explicit developer interface builds and installs a native Go image:

```sh
go build -o factory-go ./cmd/factory
./factory-go init ../new-project --local --pack go,typescript

# Select another clean checkout, including from an installed Go caller:
./factory-go init ../new-project --local --source /path/to/clean-factory --pack go
```

Commit source changes first. Tracked modifications and untracked files are
refused. An omitted source means the invoked developer checkout; an installed
caller must supply `--source DIR`. The host's provisioned Go toolchain and module
cache are required. Packaging runs offline and does not download a toolchain,
contact GitHub or call a model. The initializer may still install project
dependencies through its declared setup operations.

The exact built executable performs initialization and installs a
`local-FULL_REVISION` runtime for the native host target. This is an explicit
local build, with no publisher authentication claim. `--local --help` explains
the selector without inspecting source or compiling. Local upgrade is not
provided by this interface.

Before publication, build or validation failures preserve the source and target.
Once publication begins, an incomplete installation keeps the pending barrier
and initializer backups for recovery; automatic rollback is not promised.
Successful initialization removes its private build/staging inputs. User-owned
caches are never adopted or deleted. See [ADR 0116](../adr/0116-explicit-go-local-initialization.md).

The intended compatibility boundaries preserve user README content and unmanaged
Makefile/ignore rules. Unsafe source or destination paths and unrepresentable flat
configuration inputs must fail before installation writes. Reinitialization backups
are distinct from the migration recovery manifests; they do not establish authority
to restore or prune an installation.

## Evidence

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
