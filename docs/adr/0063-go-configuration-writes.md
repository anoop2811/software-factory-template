# ADR-0063: Candidate Go configuration writes

Status: accepted for source-candidate implementation; installed activation gated.

## Context

The G1 sourceable-library contract in specs/001-go-runtime-conversion.md:306
still lacks `factory_config_set`. Its ordinary callers record model settings,
review opt-ins and migration choices. Readers, export plans and local-hook
normalization already have Go candidates. Configuration mutation must preserve
unrelated project bytes and must not execute values as shell code.

## Decision

Under Decision 58, add private Cobra request `config set KEY VALUE`, using the
same literal identifier admission as the existing get/has protocol. Missing or
surplus operands and invalid identifiers return 2 without writes or stdout.
Add `factory_config_set` to the candidate Bash `runtime/shell/config.sh`, passing
its first two operands literally through the existing explicit runtime bridge.
Preserve missing-argument behavior under nounset and ignore surplus shell-helper
arguments as before. Sourcing the shim does not run the runtime. The installed
public library and native adapters retain their current routing.

Resolve the selected file with the existing configuration path rules, including
the setter's command-substitution removal of trailing LF from the resolved path.
Do not trim embedded LF or other whitespace. An all-LF resolved path becomes
empty and retains the missing-file diagnostic with an empty basename. Operate
only on an existing caller-owned regular file with a single hard link and
ordinary permission bits. Files with ACLs or extended attributes are outside
this source candidate qualification; do not use it to rewrite such files. Refuse a final symlink, nonregular file, special mode bits, or unreadable
input without mutation. A missing file or directory retains status 1 and the existing
`factory config: no BASENAME here — run factory init first.` diagnostic.
Other filesystem, admission and write failures return 1 with a useful diagnostic;
no usage banner, configuration value or content dump. Explicit path selection
may name a file outside the repository, just as the existing helper permits.

Rewrite bytes with the baseline literal formatting, not a general YAML encoder.
Replace every physical line beginning with the exact key and colon. Replacement
is `KEY: "VALUE"`, with LF in VALUE folded to spaces; preserve each line's LF
terminator and all unrelated bytes. A replaced line loses its former CR and
comment. Without a matching key, append `KEY: "VALUE"` followed by LF directly
to the existing bytes, retaining VALUE's LF characters. Do not invent a newline
before the append. Quotes, backslashes, ampersands and delimiters are data and
are not escaped or interpreted. These existing quirks do not promise arbitrary
quote/newline round trips through the flat reader. Regex-like keys remain outside
the candidate's existing identifier contract; no public correction is approved.

Use one Go implementation behind the adapter. Open the explicitly selected
parent directory through an `os.Root` handle, read a checked regular file and
publish a fully written replacement through an exclusive temporary file in that
same directory. Preserve its owner, group and ordinary permission bits, or
refuse before publication if that is not possible; temporary content remains
private while being prepared. Synchronize and close before rename, recheck the
original identity/content/mode before publication, and clean only the temporary
file created by this call on ordinary failure. Never use or remove a pre-existing
`.factory-bak` or similarly named sibling. Failure before rename preserves the
original file. Do not report a post-publication failure as though no write occurred.

This is an explicit single-writer source candidate: callers must keep the selected
configuration and its trusted parent directory quiescent during a write. Observed changes before publication are
refused, but this is not an atomic compare-and-swap against arbitrary external
writers and is not the mixed-runtime exclusion/recovery protocol. Reject files
larger than 16 MiB before rewriting and bound the resulting bytes to 16 MiB.
Symlink/hardlink, extended metadata, ownership-change and concurrent-writer parity
remain activation boundaries; this slice does not migrate installed state or
claim transactional installation recovery. No backup retention is advanced by
setting a configuration value.

## Observed temporary identity and cleanup

If the pre-publication check observes a different file at the temporary pathname,
cleanup must not remove that replacement. Capture the created file's identity
and compare it with `Lstat`/`os.SameFile` before ordinary failure cleanup. A missing
path needs no cleanup; a mismatched path is retained with a visible cleanup-refusal
error. An identity lookup failure must not authorize deletion. This does not
make the operation a race-free compare-and-swap against a hostile directory.

An independent collaborator test may supply a context that substitutes a sentinel
for the prepared file at a deterministic context-check boundary. It must show
that publication refuses, the original configuration survives, and the unknown
replacement remains. The test must fail before the cleanup correction; it is
separate from the compiled CLI's original outside-in RED evidence.

## Acceptance and delivery

An independent spec-writer must record outside-in Ginkgo/Gomega RED before
implementation. Compare explicit file bytes, permissions, stdout/stderr and
statuses with both immutable reader baselines for admitted ordinary files.
Exercise duplicate keys, empty/unterminated files, CRLF, quoting/metacharacters,
replacement versus append LF behavior and read-after-write. Add refusal cases
for malformed requests, missing runtime, links, special files/modes and limits;
assert originals and unrelated sentinel siblings remain unchanged on failure.
Use real compiled CLI and sourced Bash boundaries; configuration cannot create
a marker through shell-looking text or inherited helper-variable attributes.

Go source quality, shared-script checks and four-target CI remain required.
Extend the existing packaged conformance gate to exercise actual setter output,
permissions, sibling preservation and symlink refusal with no Go/Python on PATH.
An immutable pre-writer binary must fail those new cases; the four native bundle
jobs must run them against the staged binary built from the PR source.
No dependency or version changes, public installation switch, legacy deletion,
paid model calls or installer backup/cleanup behavior are included. G0/G1 remain
open until the remaining compatibility and delivery/recovery evidence passes.
