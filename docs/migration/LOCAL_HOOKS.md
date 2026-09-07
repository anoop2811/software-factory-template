# Go local-hook registration candidate

This G1 slice implements the registration parser under
[ADR-0062](../adr/0062-go-local-hook-normalization.md). It adds
`factory_local_hooks` to the source candidate readers. The installed factory
continues to use its existing scripts until compatibility, release delivery and
transactional upgrade/recovery acceptance permit activation.

## Local use

Build the candidate in the factory source checkout, then source its readers:

```sh
go build -o factory-go ./cmd/factory
FACTORY_RUNTIME_BINARY="$PWD/factory-go"
. runtime/shell/readers.sh
factory_local_hooks
```

The helper reads `local_hooks` from the selected factory configuration and prints
one hook entry per line. It does not run the hooks. For example:

```yaml
local_hooks: "scripts/check.sh --strict scripts/docs.sh"
```

prints:

```text
scripts/check.sh --strict
scripts/docs.sh
```

Use commas to keep non-flag arguments in the same entry:

```yaml
local_hooks: "scripts/check.sh --strict, scripts/docs.sh docs"
```

## Compatibility boundary

The caller shell retains its existing field splitting and pathname expansion.
Consequently, the working directory, IFS and shell glob options can affect the
result just as they do in the baseline. Text resembling command substitutions
or shell operators remains data; neither the adapter nor Go evaluates it as
shell source. Quotes embedded in a configured value do not become shell quoting.

The private `config hooks words|commas [TOKEN...]` protocol groups already
expanded operands. Words mode attaches flag tokens to the preceding hook and
ignores orphan flags. Commas mode normalizes ASCII whitespace within each field.
C/POSIX locale is the qualified reader scope; broader locale and historical
reader diagnostic boundaries still apply. The helper ignores surplus caller
arguments and leaves caller variables, IFS, options and positional arguments
unchanged. Sourcing the file does not invoke the runtime.

An explicit executable absolute `FACTORY_RUNTIME_BINARY` is required. Failed
runtime selection/calls propagate failure instead of searching PATH or falling
back to the installed parser. This candidate-only admission behavior is not an
approved change to the installed helper's historical failure masking.

## Acceptance

Independent Ginkgo/Gomega subprocess cases compare explicit outputs against
both immutable reader baselines, the compiled private protocol and the sourceable
adapter. The source gate checks the Go implementation and existing shell shim.
Recorded RED/GREEN and quality evidence follows below.

Configuration writes, unresolved reader parity, installation activation, ignored
local backups and predecessor cleanup remain separate required migration work.
No obsolete installed files are removed by this source-only slice.

## Recorded development evidence (2026-09-07)

Before implementation, the independent compiled-boundary suite failed at the
missing helper/private request. Command:

```sh
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test ./acceptance -ginkgo.focus='G1 local hook registrations' -count=1
```

```text
Ran 41 of 496 Specs in 3.119 seconds
FAIL! -- 3 Passed | 38 Failed | 0 Pending | 455 Skipped
FAIL github.com/anoop2811/software-factory-template/acceptance 3.608s
```

The three passing cases were malformed requests already refused by the bridge;
they were not implementation evidence. Review added custom-IFS empty-token
regressions and an inherited-arithmetic-variable marker test before their
respective implementation corrections. The latter demonstrated actual marker
creation from literal configuration and failed before positional-only transport.

The final suite contains 51 local-hook cases; supplementary portability and
attribute vectors were added after the initial implementation and are not claimed
as pre-implementation RED. On the final test snapshot:

```sh
rtk proxy env FACTORY_AGENT_ROLE=reviewer go test -race ./acceptance -ginkgo.focus='G1 local hook registrations' -ginkgo.no-color -ginkgo.succinct -count=1 -v
```

```text
PASS
ok  github.com/anoop2811/software-factory-template/acceptance 16.268s
```

`rtk proxy env FACTORY_AGENT_ROLE=reviewer shellcheck -s sh -S warning runtime/shell/readers.sh`
exited 0 with no output. The diff-aware configured checks also exited 0:

```sh
rtk proxy env FACTORY_AGENT_ROLE=reviewer ./scripts/hooks/diff-aware-check.sh
```

```text
selftest: 217 passed, 0 failed, 0 skipped
diff-aware-check: all 1 dispatched check(s) passed
```

The full `make go-runtime-source-check` run reached a passing race suite
(`ok .../acceptance 176.286s`), then rejected a rune-to-byte conversion at lint.
After replacing that narrowing conversion with direct rune membership, the
final focused race command (same focus as above, without `-v`) returned:

```text
ok  	github.com/anoop2811/software-factory-template/acceptance	14.073s
```

The remaining quality commands were run independently on the corrected code:

```sh
rtk proxy env PATH="/private/tmp/factory-quality-tools:$PATH" golangci-lint run --config packs/go/.golangci.yml ./...
rtk proxy env PATH="/private/tmp/factory-quality-tools:$PATH" gosec ./...
rtk proxy env PATH="/private/tmp/factory-quality-tools:$PATH" govulncheck ./...
```

All exited 0; their summaries were respectively `0 issues.`, `Issues : 0` and
`No vulnerabilities found.` This records the actual split validation sequence,
not a claim that the interrupted make invocation itself passed. Live native
model execution, release installation and recovery/cleanup are not claimed.
