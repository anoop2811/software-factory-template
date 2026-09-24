# ADR-0073: Go loop fingerprint foundation

Status: accepted for source implementation under specs/001-go-runtime-conversion.md.

## Scope

Decision 72 starts the existing loop-controller/recovery conversion package with
read-only configuration and checkpoint identity. Implement configuration, policy
digest, source snapshot and stable double-snapshot in internal/loop. The immutable
oracle is scripts/lib/loop.py at 76952eaa63aebd1ecd282f5ab51dd7c3627cb497
(configuration:29, snapshot:163, stable_snapshot:190, policy:305).

Expose only the private compiled boundary
`FACTORY_BRIDGE_PROTOCOL=1 factory loop fingerprint` with no operands. Capture
environment once. FACTORY_LOOP_ROOT selects checkout, otherwise caller cwd.
Return one JSON object containing configuration, snapshot (head/source/safety)
and policy; use current budget.Configuration for the policy's budget inputs.
Success is code 0; failures are code 2 with one sanitized stderr diagnostic and
no partial stdout. The private command never writes checkpoint/accounting state,
executes check commands, probes native CLIs or calls a model.

Installed `factory loop` stays on the current runtime. Loop history validation,
checkpoint locking/publication, manual check execution, bounded repair/review,
resume, public argument compatibility and transactional retirement remain open.
The source loop package has four milestones: fingerprint foundation; checkpoint
store/recovery invariants; manual controller/resume; bounded shared-budget
controller plus command integration. These refine one existing package, not
additional completed packages or permission to activate an incomplete stage.

## Configuration and canonical identity

Preserve existing defaults, positive finite limits, exact arbitrary-size integer
counters, strict lowercase true/false, Python float input grammar and Python
whitespace splitting for test_patterns/protected_paths. Validate test patterns
with the same system grep -E engine used by the shell hook, with literal argv
and input, preserving caller locale/environment. Never substitute Go regexp
for POSIX ERE or silently reinterpret protected fnmatchcase patterns as filepath
globs. Protected glob matching is case-sensitive and '*' may cross '/'; preserve
bracket/negation/literal-backslash behavior against the oracle.

Fingerprints are SHA256 over Python json.dumps(sort_keys=True, ensure_ascii=True,
allow_nan=False) with default comma/colon spaces. Exact bytes are contractual:
no normalization of hashes, numeric types, float spelling, Unicode escapes or
missing/null/empty values. Factor shared Python number parsing/display and a
small streaming canonical encoder into existing internal components as useful;
do not implement a second numeric grammar or marshal then parse typed config.
Keep shared budget policy validation and aggregation behavior unchanged.

Policy includes every loop and budget config field, every FACTORY_LOOP_*_MODEL
environment entry, the literal FACTORY_LOOP_CONFIG_PATH value (default empty),
and OPENCODE_CONFIG_CONTENT (default empty). Unknown unrelated environment keys
are excluded. Independent tests must vary every configuration field and each
additional policy input so omission from the derived digest cannot hide.

Keep filesystem/env raw bytes distinct from decoded JSON strings. Python
surrogateescape must preserve non-UTF8 filenames/environment when converting
them into canonical JSON; do not collapse invalid bytes into U+FFFD. Existing
jsonvalue decoded-string identity must not be reinterpreted as raw filesystem
bytes. Output contains configuration and hashes only, never source/policy file
contents, raw model/overlay environment values or probe output.

## Snapshot and filesystem behavior

Preserve inherited Git environment/config semantics and caller cwd while using
git -C checkout. Require committed HEAD, refuse unmerged entries and gitlinks,
and enumerate tracked plus nonignored untracked paths with NUL separation.
Ignore .factory and descendants. Refuse newline/carriage-return names as the
baseline does. Hash arbitrary file bytes and full stat.S_IMODE bits (07777),
represent deleted paths as null, and hash symlink target bytes rather than the
referent. Index-only changes do not alter a worktree-content fingerprint.

Safety combines governing source entries (fixed instructions/config directories,
protected paths and matched test paths), native_policy and effective config.
Preserve the baseline's known native file/tree inventory, missing file nulls,
absent/present tree booleans, 512 entry/file checks, 1 MiB per-file limit and
8 MiB total. Native policy refuses symlinks including ancestors, nonregular files
and oversized/growing reads. Hardlinks remain allowed. Effective config may be
ignored or outside the checkout; a relative explicit path uses caller cwd and
symlink content is its target text. Preserve lexical symlink/.. path semantics.

StableSnapshot performs two full observations and refuses any difference. No
cache or reuse of the first observation may hide intervening edits. Snapshots
are evidence of observed cooperating local state, not an atomic filesystem
transaction or a hostile same-user replacement security boundary. Uncertainty
must refuse instead of authorizing future work with a partial fingerprint.

## Private resource qualification

The Python source scanner has unbounded file/Git-output reads. This private Go
candidate explicitly qualifies at most 100000 source entries, 64 MiB per source
file, 512 MiB total source bytes per snapshot, and 32 MiB output per Git/grep
probe. Enforce limits during reads, not only from stat. These are documented
private refusal boundaries requiring review before public activation, not claims
of parity for larger repositories. Native-policy limits remain the baseline's.

Git probes have ten-second deadlines, grep probes five seconds, shortened by
caller cancellation. Use literal exec argv, bounded output, sanitized errors and
bounded pipe cleanup. Owned probe process groups must not survive timeout or
cancellation unnoticed. Preserve parent cwd and captured environment; do not
reuse packaging's intentionally sanitized Git environment or harness launch
policy for this different boundary. Source hashing checks context between read
chunks. No arbitrary filesystem syscall deadline is promised.

Regular-file opens must not turn a concurrent replacement into an unbounded
FIFO read or follow a replacement symlink. This fail-closed correction is tested
separately from stable-tree parity; hostile filesystem transactions remain out
of scope. Native policy read bounds must hold after a stat/open size change.

## Independent acceptance

Before implementation, independently authored Ginkgo/Gomega cases fail through
the compiled private CLI for its missing fingerprint operation. Compare exact
hashes to the immutable Python oracle on isolated real Git repositories; no
provider/native CLI calls. Cover configuration defaults/invalid inputs, numeric
identity and Unicode, every policy field, source edits/modes/deletion/symlinks,
ignored policy, external/relative config, protected globs/ERE, .factory exclusion,
HEAD/index changes, conflicts/gitlinks/newline names, missing/empty policy trees,
resource bounds and cancellation. Use private collaborator seams only where
needed to deterministically prove mutation between observations or read races.

Compare JSON configuration structurally with exact integer values and float
values, allowing outer object key order/number spelling only; compare all three
snapshot fields and policy digest exactly. Human failure wording may differ,
but code, streams, no-effects behavior and sanitized diagnostics must hold.
No Python interpreter is used by the Go implementation.

## Sources

Fetched 2026-09-23 (America/Los_Angeles): https://docs.python.org/3/library/json.html documents sorted
keys, ASCII escaping and default separators;
https://docs.python.org/3/library/fnmatch.html documents shell-style matching;
https://docs.python.org/3/library/os.html documents filesystem surrogateescape.
https://pkg.go.dev/os/exec#Cmd documents CommandContext, Cancel and WaitDelay.
No dependency or toolchain version is added or changed.

Native-policy aggregate accounting also checks actual bytes read, so multiple
post-stat growing files cannot exceed 8 MiB despite small earlier size reports.
This is an explicit fail-closed correction to the baseline's stat-only aggregate
check, with deterministic growth evidence before public activation.

The private fingerprint command handles SIGINT and SIGTERM by canceling its
context and completing owned probe cleanup before returning code 2 with a
sanitized diagnostic. This signal handling belongs only to this private command;
other bridge and public command signal contracts remain unchanged. Acceptance
must observe a started probe, interrupt the compiled CLI, and observe that its
owned probe no longer runs. Uncatchable SIGKILL is outside this guarantee.
API source fetched 2026-09-23 (America/Los_Angeles): https://pkg.go.dev/os/signal#NotifyContext.
