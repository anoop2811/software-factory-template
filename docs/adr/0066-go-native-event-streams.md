# ADR-0066: Go native event-stream accounting candidate

Date: 2026-09-08 UTC
Status: implementation slice under the approved conversion

## Scope

Add private `FACTORY_BRIDGE_PROTOCOL=1 factory-go usage parse HARNESS` to consume
raw native stdout and emit the same metadata object as usage normalize. Use the
immutable Python parse_events and normalize functions at
`a0216779e4f94057f3e6752ce0ef178499721af4` as the compatibility oracle.
This advances specs/001-go-runtime-conversion.md:308 (FR-005) without launching
native CLIs or changing installed adapters. Legacy parsing buffers bounded raw
stdout before interpretation; this command must not invent incremental event
publication or a new harness wire format. Chunk boundaries must not affect output.

## Admission and result

Admit exactly one supported harness (codex, claude, opencode) before reading stdin;
invalid command/arity/harness returns status 2. Read at most 16 MiB plus one byte
for overflow detection. I/O failure, cancellation, or overflow returns status 1
with `factory bridge: invalid usage stream input` and no stdout. Metadata output
failure returns status 1 with `factory bridge: cannot write usage metadata`.
Malformed bytes/JSON are accounting data, not invocation errors: preserve the
legacy malformed-event sentinel and return status 0 with normalized unknown or
incomplete metadata. No input, transcript, credential or arbitrary event fields
may enter diagnostics or output. Successful output is one strict JSON object
and LF, with the existing five fields and numeric accounting semantics.

## Parsing compatibility

Decode all bytes as strict UTF-8 first; any decoding error yields one malformed
sentinel and discards all events. Attempt whole-document Python JSON decoding
before line decoding. A whole object yields one event; any other successful JSON
value (including an array) yields one malformed sentinel. On whole-document
syntax failure, split using Python str.splitlines boundaries, skip lines where
Python str.strip is empty, decode each remaining line independently, and retain
non-object JSON values and malformed lines as incomplete-accounting evidence.
An empty result yields one malformed sentinel. Pretty-printed whole objects,
CRLF, final unterminated lines, duplicate keys (last wins), arbitrary reader
chunking, and JSON-whitespace versus Unicode line/strip distinctions must match.

Preserve Python JSON extensions NaN, Infinity and -Infinity as non-finite numeric
values, not malformed syntax that can erase otherwise valid event fields.
Preserve Python decoded numeric distinctions: integer literals versus floats,
float overflow/underflow, and escaped Unicode including surrogate-pair equality
and distinct unpaired surrogates. Never merge distinct OpenCode identities by
replacing lone surrogates with U+FFFD. Reuse shared accounting semantics after
parsing; do not serialize permissive values through strict JSON to normalize.

For bounded private admission, reject JSON nesting deeper than 512 with status 1
and the fixed input diagnostic. This safety boundary is specific to the private
candidate; it does not authorize changing the installed Python controller.
Qualify decimal integer decoding against Python's standard 4300-digit limit:
an integer token beyond 4300 digits is a JSON decoding failure (line fallback or
malformed sentinel), including when it occurs in an otherwise ignored field.
Pin this oracle setting explicitly in acceptance; alternative interpreter
integer-limit overrides are outside this private candidate's qualification.
The existing strict usage normalize command and its contract remain unchanged.

## Delivery and acceptance

Use Cobra and outside-in Ginkgo/Gomega acceptance before implementation. Compare
compiled output to explicit expectations and immutable Python parse+normalize
results. Include all three harnesses, framing/split boundaries, invalid UTF-8,
non-object/malformed events, empty streams, partial lines, exact integers and
float forms, permissive constants, Unicode identity collisions, oversized and
deep inputs, fixed diagnostics, and no subprocess/network/state dependencies.
Include packaged native conformance with Python and Go absent from runtime PATH.
Do not pin a new dependency or change native model/tool versions for this slice.

Response-text extraction, native process supervision, live CLI qualification,
interoperable locks, state writes, installed runtime cutover, and retirement are
separate acceptance boundaries. Keep the agreed gitignored recovery-folder and
retention policy for installed migration; no scripts are retired by this candidate.

Authoritative references fetched 2026-09-08 UTC:
- https://docs.python.org/3/library/json.html — integer conversion limits,
  permissive constants and string decoding behavior.
- https://docs.python.org/3/library/stdtypes.html#str.splitlines — line boundaries.
- https://pkg.go.dev/encoding/json#Unmarshal — replacement of invalid UTF-16
  surrogate pairs explains why default string decoding cannot preserve identity.
