# Native Go initialization

The Go CLI's ordinary `init` selects an authenticated packaged Go initializer.
The source checkout's tracked shell launcher and the public installer's stable
v0.1.6 pin retain their existing script behavior until their separate activation.
Building the Go command does not change either entrypoint:

```sh
go build -o factory-go ./cmd/factory
./factory-go init /path/to/project --pack go,typescript
```

Keep the tracked `factory` file intact. Ordinary Go initialization authenticates
the selected release's complete assets, then runs that exact initializer. It
does not copy the caller's source templates or require a Go compiler or Python
at installation time. Git, GitHub CLI authentication/attestation support and
the tools required by retained gates and project setup are still needed.

Selection order is `--ref TAG`, nonempty `FACTORY_REF`, the admitted installed
release version, then the prepared source default `v0.2.0`. Explicit and environment
refs must be exact stable Go tags at or after v0.2.0; `main`, old Bash tags and
implicit prereleases refuse. An installed official prerelease can reuse its own
version. A local installation must explicitly choose local source or an official
release. For a published prerelease, use `--go-candidate TAG` instead:

```sh
./factory-go init ../new-project --go-candidate v0.2.0-rc.3 --pack go
./factory-go init --help
```

The prepared v0.2.0 default is not a publication claim. Until that release exists,
ordinary source-built init refuses without changing the target. `--help` performs
no acquisition or target writes. Put a target beginning with an option name after
`--`; it stays literal through version selection and authenticated handoff.
Explicit candidate and local selectors take precedence over ambient defaults.
See [ADR 0118](../adr/0118-default-packaged-go-initialization.md).

The initializer collects project settings, waits for confirmation, installs the
selected packs and factory assets, synchronizes OpenCode, Claude Code and Codex,
and requires the installed gate selftests to pass. It does not invoke a model.
Dependency installation through npm and declared shared operations remain
external boundaries. A failed initialization is not a completed installation;
once publication starts, preserve its pending barrier and backups for recovery.
No automatic initializer rollback is claimed. Existing installation upgrades,
migration recovery, rollback and retention remain separate commands.

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
the selector without inspecting source or compiling. For updating an existing
installation, use the separate [local upgrade interface](../UPGRADING.md).

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

## Historical initializer component evidence

The following observations qualify the original native initializer component.
They do not establish current authenticated default selection. Its historical
prompt, data, pack and publication cases remain explicit component qualification;
current packaged delivery has separate public CLI acceptance scenarios.

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
