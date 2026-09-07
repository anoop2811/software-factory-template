# ADR-0064: Go native usage accounting candidate

Date: 2026-09-07
Status: implementation slice under the approved Go conversion

## Decision and scope

Start G2 with the metadata-only normalization boundary from
`scripts/lib/budget_adapters.py` at immutable merged revision
`49636db85bfb58eeafa761fb99000a12420bcb43`. This supports
specs/001-go-runtime-conversion.md:308 and specs/001-go-runtime-conversion.md:310
(FR-005 and FR-007), and AC 2.1 at specs/001-go-runtime-conversion.md:123.
It does not launch native CLIs or replace the installed Python controller.

## Private request

`FACTORY_BRIDGE_PROTOCOL=1 factory-go usage normalize HARNESS` consumes exactly
one JSON array of decoded events on stdin. HARNESS is one of `codex`, `claude`,
`opencode`; unknown harnesses or wrong operand counts return status 2 before
reading stdin. Input is bounded to 16 MiB and must be valid UTF-8, strict JSON,
and an array. Invalid input, excessive size, or read/output failure returns
status 1 with a fixed diagnostic that never includes input content. No output
metadata is emitted on input failure. Input failures emit exactly
`factory bridge: invalid usage event input` plus LF; output failures emit
`factory bridge: cannot write usage metadata` plus LF. A valid array, including an empty array
or non-object event members, returns status 0 with one JSON object and newline.
Duplicate object keys retain the last value, matching the baseline JSON decoder.
Status 1 on output failure refers to returned I/O errors; normal process signal
behavior (including SIGPIPE for a closed stdout pipe) is not overridden.

The array is the boundary after stream decoding, not a new native wire format.
The Python stream parser, nonstandard NaN/Infinity spellings, raw NDJSON decoding,
unpaired Unicode surrogate handling and live CLI version qualification are
outside this slice. Strict input refusal does not authorize a public behavior
correction. No automatic input files, configuration, ledger, locks or credentials
are read or written. No external command, network request or paid call occurs.

## Metadata contract

Emit exactly `tokens`, `estimated_usd`, `complete`, `failed`, `source`, with their
baseline meanings. Absent usage/cost is JSON null, not zero. Never emit transcript,
answer text, prompts, credentials, provider error strings or unknown event fields.
Integer token counts retain their exact value, including values beyond float64's
exact-integer range; reject booleans, fractional/exponent-encoded token numbers,
negative values and numbers whose Python finite check would overflow. Numeric
costs retain baseline finite/nonnegative semantics and float summation behavior.

Codex selects the last `turn.completed` usage, requires input/cached-input/output
integer tokens, retains optional reasoning tokens, flags `turn.failed`/`error`,
and always leaves cost unknown. Completeness also requires no failure and no
non-object event. Claude selects the last `result`, requires the four baseline
usage counters, and retains finite client cost except for `error_during_execution`
or malformed events. Its failed and complete fields are independent: a failed
result can still have complete reported accounting. Preserve that distinction.

OpenCode sums unique `step_finish` parts by the exact three-string identity
(sessionID, messageID, id). All strings must be nonempty. Required input/output/
reasoning and cache read/write counters must be nonnegative integer numbers;
optional total is validated but not emitted. Equal duplicates do not add cost
or tokens; conflicting duplicates invalidate accounting. Signature equality
matches Python numeric/structural equality for the decoded JSON domain. Unknown
fields do not change signatures. Signature comparison precedes validation of a
duplicate cost: a valid first cost `1` and equal-identity duplicate cost `true`
compare equal in Python and do not invalidate the prior accounting. Token/cache
fields are validated while constructing signatures, before that comparison.
Only valid, failure-free input ending in a
unique step with reason `stop` has complete accounting. Preserve zero cost,
null unknowns, floating cost-sum overflow refusal, and all five normalized token
counters. Integer token sums stay exact even when their aggregate exceeds the
finite-conversion range used to admit each individual counter.
Any non-object event clears completeness and estimated cost for every harness.

## Delivery and evidence

Implement one shared Go package behind Cobra, with no per-harness shell parser.
All three native adapters remain installed as-is until supervision, stream
parsing, mixed-runtime exclusion and recovery pass their own acceptance gates.
No runtime is activated, no Python file is retired, and no legacy backup is
created by this source candidate. Gitignored recovery/retention and active-code
cleanup remain mandatory at installed cutover under the migration spec.

An independent spec author first exercises the compiled private CLI against
explicit expected metadata and the immutable Python normalizer. Cover all three
harnesses, missing/failed/malformed usage, numeric boundaries, duplicates,
privacy, literal content, no side effects, refusal status and no external tools.
Packaged conformance must exercise all three harnesses against actual staged
binaries on Linux/macOS amd64/arm64. The local Python oracle is test-only and
must not be required on the packaged candidate's runtime PATH. Record RED/GREEN
and quality evidence separately; this slice does not complete G1 or G2.
