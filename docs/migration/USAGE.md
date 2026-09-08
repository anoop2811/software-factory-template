# Go native usage accounting candidate

This G2 slice implements [ADR-0064](../adr/0064-go-native-usage-accounting.md):
shared metadata normalization for Codex, Claude Code and OpenCode. The installed
budget and loop commands continue to use their existing controllers. This
candidate does not launch a model or change a spending limit.

## Try the source candidate

Build with the development toolchain pinned in `go.mod`:

```sh
go build -o factory-go ./cmd/factory
printf '%s\n' '[{"type":"turn.completed","usage":{"input_tokens":12,"cached_input_tokens":2,"output_tokens":3}}]' |
  FACTORY_BRIDGE_PROTOCOL=1 ./factory-go usage normalize codex
```

The private command accepts a JSON array of already-decoded events on stdin,
not a raw native NDJSON stream. Select `codex`, `claude` or `opencode` explicitly.
It emits only token counts, client-estimated cost, completeness, failure status
and a fixed source label. JSON null means unknown; zero means reported zero.
Transcript, answer text and provider errors are not copied into the result.

Codex cost stays unknown. Claude's failure and accounting completeness are
separate fields. OpenCode sums unique step identities and refuses complete
accounting for conflicting duplicates or a missing final stop. These distinctions
preserve the immutable Python baseline rather than inventing a common billing
model. Client estimates are not authoritative provider invoices.

Input is limited to 16 MiB, valid UTF-8 and one strict JSON array. Invalid input
returns status 1 without metadata; unsupported harnesses and invalid operands
return status 2. Error text never includes event content. No files, ledger,
configuration, credentials or subprocesses are needed by this private command.
Nonstandard JSON constants and unpaired Unicode surrogate handling remain
outside the qualified compatibility domain; see the ADR for all boundaries.

## Remaining conversion

This is the accounting component, not the process supervisor or budget admission
controller. The [raw stream candidate](STREAMS.md) extends metadata parsing separately.
Response display, native preflight, deadlines,
shared legacy/Go locks, history publication and loop recovery remain to be
ported. Installed activation requires the migration's delivery, recovery and
manual pilot gates. Superseded Python files will be retired only with their
replacement, gitignored recovery copies and predecessor cleanup in place.

## Development evidence

The initial independent compiled-boundary run preceded implementation. All six
explicit success/empty cases passed their immutable Python oracle comparisons,
then failed against the missing Go private command:

```sh
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test ./acceptance -ginkgo.focus='G2 native usage normalization' -ginkgo.no-color -ginkgo.succinct -count=1
```

```text
Ran 6 of 578 Specs in 1.926 seconds
FAIL! -- 0 Passed | 6 Failed | 0 Pending | 572 Skipped
FAIL github.com/anoop2811/software-factory-template/acceptance 2.309s
```

Later supplemental cases are not part of that initial RED record. The immutable
normalizer oracle is test-only; packaged conformance runs without Python or Go
on the candidate's runtime PATH.

The final focused suite has 93 cases, including strict input admission, fixed
error output, all three harnesses, exact counters, numeric underflow/overflow,
duplicate ordering and privacy. The test author's command:

```sh
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test -v ./acceptance -ginkgo.focus='G2 native usage normalization' -ginkgo.no-color -ginkgo.succinct -count=1
```

```text
93/665 specs
PASS
ok github.com/anoop2811/software-factory-template/acceptance 29.197s
```

Three subsequently added packaged cases failed against the immutable pre-feature
binary built from `49636db85bfb58eeafa761fb99000a12420bcb43`:

```sh
rtk proxy env FACTORY_AGENT_ROLE=spec-writer FACTORY_BUNDLE_BINARY=/var/folders/83/yj7qqyt551xbbpvm54tqkcbw0000gn/T/factory-usage-baseline-s15n69v7/factory-go go test -v ./acceptance -ginkgo.focus='Packaged runtime conformance.*normalizes native usage' -ginkgo.no-color -ginkgo.succinct -count=1
```

```text
Ran 3 of 668 Specs in 0.711 seconds
FAIL! -- 0 Passed | 3 Failed | 0 Pending | 665 Skipped
FAIL github.com/anoop2811/software-factory-template/acceptance 1.327s
```

All eight packaged-conformance cases passed with the current local build:

```sh
rtk proxy env FACTORY_AGENT_ROLE=spec-writer FACTORY_BUNDLE_BINARY=/private/tmp/factory-usage-candidate go test -v ./acceptance -ginkgo.focus='Packaged runtime conformance' -ginkgo.no-color -ginkgo.succinct -count=1
```

```text
PASS
ok github.com/anoop2811/software-factory-template/acceptance 1.056s
```

Those paths identify ephemeral local fixtures, not authenticated release assets.
Native bundle CI supplies actual staged binaries to this same conformance suite.
A test-owned subprocess helper has a call-scoped G702/G204 annotation. A G101
false positive on a local JSON-counter fixture was resolved by renaming the
variable `expectedCounters`, with no assertion or production-scanning changes.

Full source gate on the implemented slice:

```sh
rtk proxy env PATH="/private/tmp/factory-quality-tools:$PATH" make go-runtime-source-check
```

```text
ok github.com/anoop2811/software-factory-template/acceptance 204.661s
0 issues.
Issues : 0
No vulnerabilities found.
```

The Linux-target linter and configured diff-aware checks also exited 0:

```sh
rtk proxy env GOOS=linux GOARCH=amd64 /private/tmp/factory-quality-tools/golangci-lint run --config packs/go/.golangci.yml ./...
rtk proxy env FACTORY_AGENT_ROLE=reviewer ./scripts/hooks/diff-aware-check.sh
```

```text
0 issues.
selftest: 217 passed, 0 failed, 0 skipped
diff-aware-check: all 1 dispatched check(s) passed
```

Independent correctness/test and security review found no reproducible defect.
Eight additional reviewer numeric/deduplication CLI probes matched the immutable
baseline. These results do not qualify live native CLI behavior, installed
activation, recovery or legacy cleanup.
