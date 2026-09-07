# Read-only Go reader candidate

This source-only slice implements configuration path/get/has and role-tier
resolution under Decision 51 (docs/DECISION_LOG.md:1952), following the
sourceable-library contract in specs/001-go-runtime-conversion.md:306.
The installed `factory`, configuration libraries and three harness adapters
continue to use their existing implementations. No installer, runtime activation
or legacy asset retirement is part of this slice. G0/G1 acceptance remains open.

## Local use

Build in the source checkout using the pinned Go version:

```sh
go build -o factory-go ./cmd/factory
FACTORY_RUNTIME_BINARY="$PWD/factory-go"
. runtime/shell/readers.sh
factory_config_get project_name fallback
factory_config_has project_name
role_tier reviewer
resolve_tier economy economy
```

The shim needs an explicit absolute executable file path. It does not search
PATH, build, download or fall back to old parsing. Sourcing it only defines
functions; runtime selection happens when a helper is called. Each call passes
literal arguments to one child process without evaluating returned shell text.
The parent working directory, environment and shell options are preserved.
A caller's non-exported or readonly `FACTORY_CONFIG` override is passed only
to the child. `/usr/bin/env` is required for child-only environment assignment
without changing readonly parent variables; no runtime executable is found
through PATH.

## Private protocol

`FACTORY_BRIDGE_PROTOCOL=1` selects a separate Cobra tree in the same binary.
This developer protocol is not a new public command surface. An empty or absent
protocol variable retains ordinary CLI dispatch; unknown versions refuse with
status 2. The read-only requests are listed below; the additional export/legacy
requests are documented in [the export candidate](EXPORTS.md).

| Request | Result |
| --- | --- |
| `config file` | Exact nonempty `FACTORY_CONFIG`, otherwise Git root plus `/factory.yaml`, otherwise `./factory.yaml` |
| `config get KEY [DEFAULT]` | First flat matching value; missing/empty value returns the literal default |
| `config has KEY` | Status 0 for a matching physical key, 1 when absent |
| `role tier ROLE` | Frontier for spec-writer/reviewer, economy for refactorer/wiki-maintainer, default otherwise |
| `role resolve PROFILE TIER` | Economy becomes default unless profile is exactly economy; other tiers pass through |

Keys are literal `[A-Za-z_][A-Za-z0-9_-]*` identifiers. Unknown requests and
invalid arity return 2 on stderr without unsolicited help or completion.
Positional values such as `--help` remain data. Helper adapters preserve the
legacy ignored-surplus-argument behavior independently of strict private arity.
Missing runtime also returns 2; configuration read errors preserve get's default
and status 0, while has returns 2 with a diagnostic. Missing/nonregular config
files retain the legacy missing-value behavior. Regular symlinks are followed.

## Compatibility boundaries before activation

The parser intentionally implements the historical flat format, not general
YAML: first physical match, existing double-quote and inline-comment rules,
ASCII whitespace, and Bash's NUL removal within the selected value. Values over
64 KiB and literal shell metacharacters are covered without invoking a shell
parser. Git is used only for the existing repository-root lookup.

C/POSIX locale behavior is the current candidate contract. Historical sed/grep
Unicode whitespace classification varies by locale and operating system;
non-C parity remains unresolved. Regex-like legacy key queries and missing-key calls remain outside the private
identifier contract. In particular, legacy `factory_config_has` can return 1 for
a missing configuration before expanding a missing key, even under nounset.
Required-operand failures against an existing configuration are covered; exact
shell diagnostic framing and deferred-expansion cases remain open before
installed adapters can change. The candidate does not force the parent locale.
Read diagnostics retain file/reason/status but differ in sed/grep versus Go
framing; Linux Bash may additionally warn about discarded NUL bytes. These are
open compatibility boundaries, not approved behavior corrections.

The [configuration export candidate](EXPORTS.md) adds a separate Bash adapter
for export and legacy loading. Writes and local-hook tokenization remain
subsequent slices. Packaging, artifact trust, recoverable upgrades and manual
adopter qualification must pass before default activation. That later work must
move owned superseded assets into ignored local recovery storage and apply the
approved predecessor retention/cleanup rules; this candidate removes no assets.

## Acceptance

Independent Ginkgo/Gomega subprocess tests compare both immutable baselines
listed in the compatibility inventory with separately specified expected
results, then exercise the compiled Go binary and POSIX-sourceable shim.
They use local fixtures and make no paid native-model calls.

`make go-runtime-check` includes the compiled acceptance suite and POSIX shim
syntax. Template CI also runs POSIX shellcheck. Public dispatcher tests remain
in the same suite to detect private-protocol leakage into existing commands.

## Development evidence (2026-09-06)

For AC 1.2 (inert configuration, specs/001-go-runtime-conversion.md:98), AC 1.1
(sourceable entrypoints, specs/001-go-runtime-conversion.md:91), and AC 6.4
(outside-in evidence, specs/001-go-runtime-conversion.md:285), the independent
compiled-boundary RED preceded the reader implementation:

```text
$ go test ./acceptance -ginkgo.no-color -ginkgo.focus='G1 private read-only'
Ran 40 of 216 Specs in 8.610 seconds
FAIL! -- 0 Passed | 40 Failed | 0 Pending | 176 Skipped
```

Review added required-argument and readonly-parent regressions before the shim
correction. The existing configuration fixture passed both historical baselines
before these six cases failed against the candidate:

```text
$ go test ./acceptance -ginkgo.no-color -ginkgo.focus='missing required configuration operands|readonly parent configuration'
Ran 6 of 287 Specs in 1.356 seconds
FAIL! -- 0 Passed | 6 Failed | 0 Pending | 281 Skipped
```

The role-nounset regressions likewise failed before the correction:

```text
$ go test ./acceptance -ginkgo.no-color -ginkgo.focus="missing-operand behavior"
Ran 6 of 280 Specs in 2.306 seconds
FAIL! -- 3 Passed | 3 Failed | 0 Pending | 274 Skipped
```

After the correction, the combined 12-case focused regression exited 0:

```text
$ go test ./acceptance -ginkgo.no-color -ginkgo.focus='missing required configuration operands|readonly parent configuration|role helper missing-operand'
ok  github.com/anoop2811/software-factory-template/acceptance 2.380s
```

Isolated source-gate negative controls rejected a missing shim and a malformed
POSIX shim. The actual candidate passed `sh -n runtime/shell/readers.sh` and
`shellcheck -s sh -S warning runtime/shell/readers.sh` with exit 0 and no output.

The final regression suite contains 290 scenarios (114 reader scenarios).
`make go-runtime-check` exited 0 after the final test changes, including
`go test -race -count=1 ./...`, the rooted Ginkgo dialect gate, vet, build, lint,
security and vulnerability checks:

```text
$ make go-runtime-check
ok  	github.com/anoop2811/software-factory-template/acceptance	58.540s
0 issues.
  Issues : 0
No vulnerabilities found.
```

`make eval` also returned `harness-structural-eval: PASS` for opencode, claude and
codex. This is deterministic adapter-wiring evidence; live native-model runs,
packaged installation, recovery/cleanup and full stage acceptance are not claimed.
