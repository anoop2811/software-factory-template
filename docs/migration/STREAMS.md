# Go raw native event accounting candidate

This G2 slice follows [ADR-0066](../adr/0066-go-native-event-streams.md). The
private `usage parse` command accepts raw captured stdout for Codex, Claude Code
and OpenCode, then returns the accounting metadata established by `usage normalize`.
It does not publish intermediate events or run a native CLI.

## Source candidate

```sh
go build -o factory-go ./cmd/factory
printf '%s\n' '{"type":"turn.completed","usage":{"input_tokens":12,"cached_input_tokens":2,"output_tokens":3}}' |
  FACTORY_BRIDGE_PROTOCOL=1 ./factory-go usage parse codex
```

The parser tries a whole JSON object first, then Python-compatible line splitting
when whole-document syntax fails. A whole JSON array is malformed accounting,
not an event array; use `usage normalize` for already-decoded arrays. Malformed
raw data produces unknown/incomplete metadata with status 0. Private admission
failures (over 16 MiB, excessive nesting, input/output errors) return status 1;
unsupported harnesses/operands return status 2 before reading input.

Only the five accounting fields are emitted. No transcript or response text is
returned. Python-compatible number and Unicode parsing matters even for metadata:
non-finite unknown fields must not erase a valid event, and distinct escaped
surrogates must not collapse OpenCode identities. The private strict-array
normalizer retains its separate input contract.

## Remaining boundaries

This is bounded raw-output interpretation after collection, matching the legacy
parser placement. Reader chunks may split lines or UTF-8 sequences; they are not
independently published. Native process supervision, response-text extraction,
preflight, state publication, mixed-runtime locks and installed activation still
need their own acceptance. No Python script is retired in this candidate. The
migration's gitignored recovery copies and retention/pruning policy remain
required when installed implementations are replaced.

## Evidence

Initial independent RED used:

```sh
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test -v ./acceptance -ginkgo.focus='G2 native event stream accounting' -ginkgo.no-color -ginkgo.succinct -count=1
```

The six initial whole-object and whole-array cases first matched explicit
expectations through the immutable Python oracle, then failed against the absent
Go command: `Ran 6 of 674 Specs`, `0 Passed | 6 Failed` (acceptance `2.494s`).
The expanded pre-implementation suite then reported `Ran 78 of 746 Specs`,
`6 Passed | 72 Failed` (acceptance `23.235s`). Existing invalid-operand refusals
account for the passing cases; stream behavior remained red.
The final focused command above passed all 78 stream cases (`30.222s`).

Packaged negative control used the immutable pre-feature executable:

```sh
rtk proxy env FACTORY_AGENT_ROLE=spec-writer FACTORY_BUNDLE_BINARY=/private/tmp/factory-stream-prefeature go test -v ./acceptance -ginkgo.focus='Packaged runtime conformance.*parses raw native streams' -ginkgo.no-color -ginkgo.succinct -count=1
```

All three new packaged cases failed on its missing command (`1.268s`). Replacing
that executable with the current source-built candidate and selecting
`-ginkgo.focus='Packaged runtime conformance'` passed all 11 packaged cases
(`1.352s`). These are local binary checks; native staged artifacts are exercised
by the existing four-target CI matrix. They do not establish signed release or
installed-runtime qualification.

Independent review found no concrete issue. Fifty targeted compiled probes
matched the immutable baseline across malformed tokens/escapes, Python splitlines,
permissive numeric duplicates and surrogate identity distinctions. The depth
boundary returned status 0 at 512 and the fixed status-1 diagnostic at 513.
The documented source example returned status 0 and the expected Codex metadata.

`rtk proxy env FACTORY_AGENT_ROLE=reviewer ./scripts/hooks/diff-aware-check.sh`
reported `selftest: 217 passed, 0 failed, 0 skipped` and
`diff-aware-check: all 1 dispatched check(s) passed`.


The complete Go source gate ran with:

```sh
rtk proxy env FACTORY_AGENT_ROLE=reviewer PATH="/private/tmp/factory-quality-tools:$PATH" make go-runtime-source-check
```

It exited 0. Output included:

```text
ok  	github.com/anoop2811/software-factory-template/acceptance	237.602s
0 issues.
Issues : 0
No vulnerabilities found.
```
