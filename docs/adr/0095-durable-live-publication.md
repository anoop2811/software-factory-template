# ADR 0095: durable live publication completion and inert records

- Status: accepted implementation refinement; no installed activation consumer
- Date: 2026-10-04 UTC
- Decision: 94

## Purpose and remaining scope

After merged PR #121, implement R2.1: durable live completion and inert restart
inspection. R2.2 remains qualified recovery after interruption. R3 remains the
public rollback and compatibility integration. A record cannot recreate the
opaque live capability or turn arbitrary recorded bytes into known references.
This refines specs/001-go-runtime-conversion.md:161 and
specs/001-go-runtime-conversion.md:321. It does not close AC 3.2 or authorize a
default runtime switch.

The trusted future installer still owns target qualification, complete dependency
and installation coverage, state compatibility, unbridged-process quiescence and
activation checks. Finish below completes one file publication, not an installed
runtime or a successful release. ADR 0094's prohibition on forward completion is
superseded only for the new durable handle and this bounded source behavior.

## Additive source API

Add `BeginDurablePublication(ctx, root, request, environment)` using the existing
PublicationRequest and opaque Publication lifecycle. Preserve BeginPublication,
its empty pending marker and all existing Apply/Restore/Close behavior.

Add `Publication.Finish(ctx)` for a durable handle only. A legacy live handle,
nil/zero handle, closed/finished handle, unapplied original or reverse operation
cannot finish forward. Apply/Restore/Finish/Close share the existing private
mutex and ownership, including ordinary Go value aliases.

Add `InspectPublications(ctx, root)` returning bounded structured observations.
It creates no storage, invokes no processes and grants no activation, rollback,
restoration, pruning or reconstructed ownership authority. Its authority fields
remain false, including for a terminal record alongside pending evidence.

The report has `schema_version`, `mode` (`inspect_publications`), `root_status`,
`complete` (inspection completeness only), `pending_status`, `records`, and
`record_count`. The authority fields `restorable`, `rollback_ready`,
`activation_ready`, `applicable` and `prune_authorized` are always false.
Each public record has migration/operation identifiers, selected path,
classification, phase, outcome and next action. `Status()` returns 1 for I/O,
2 for unsafe/unrecognized/incomplete evidence or a present pending blocker,
and 0 for a complete inert inspection without that blocker. Observed terminal
outcome does not establish that barrier removal or an installation completed.

Only the new constructor may reuse the existing bounded read-only local Git
supervisor to establish effective exclusion and untracked storage before journal
creation. Apply/Restore/Finish/Close remain filesystem-only. No public command, check,
model, network request, background work or new dependency is introduced.

The constructor must establish its actual durable empty pending entry before any
Git subprocess starts. The retained exclusive guard alone has no durable activity
entry: a failed query can leave unconfirmed process-group ownership. Constructor
failure preserves pending even before target mutation, so releasing its lock
cannot admit cooperating work while that group may survive. A known occupied
record may be refused through inert filesystem preflight before pending creation;
that preflight launches no process. No forced cleanup follows a failed query.
If query process-group exit is unconfirmed, preserve its actual PID through the
existing typed native OwnershipError in the returned error, including cancellation
and other joined failures. A generic diagnostic must not erase owned uncertainty.

## Reserved inert storage and containment

Use `.factory/backups/.publications/MIGRATION_ID.json`: one bounded record slot
per selected existing recovery set. The private 0700 reserved directory is a
sibling namespace, never inside the immutable v1 set. Do not rewrite a saved
manifest, hold or payload. Initially refuse every occupied record slot, even if
its local JSON looks valid; new transactions do not overwrite earlier receipts.
Same-handle retries reuse only that handle's actual owned slot.

Before any new record is written, qualify actual Git ignoring and absence from
the index for the reserved root and intended slot. Reuse fixed Git configuration
and environment sanitation. Preserve ignore files and the index; missing or
negated exclusion is a refusal rather than an automatic rule change. The
existing `/.factory/backups/` exclusion covers this namespace only when Git
confirms it is effective. Require that exact rule in the safely pinned local
exclude file; read back and sync the unchanged file and `.git/info` before any
journal write, then recheck actual ignoring and tracked status. An absent rule
is a refusal, even if a broad project rule currently hides the path. Visibility
alone does not prove durability. No manifest or caller boolean proves either.

The existing backup root already has explicit exclusion from snapshot/context,
hook/test discovery and release packaging. Keep that boundary; add no separate
active namespace. Recovery inventory must recognize only this exact reserved
metadata namespace, validate its private ordinary directory and bounded contents,
and never misrepresent it as a saved installation set. Unsafe or unknown contents
are reported/preserved, not skipped behind an integrity success claim. Preserve
the existing maximum of 64 saved sets and bound records at 64 separately.

Revalidate physical root, controlled ancestry, complete ordinary modes, ownership,
single-link file identity and descriptor/name agreement. Reject links, special
files, changed ancestors, hard links, oversized entries and unsafe names without
touching referents. Incomplete constructor storage stays inert and is reported;
it never becomes automatic execution or deletion authority.

## Record contract and actual ownership

Use a closed versioned UTF-8 JSON schema within 64 KiB. Reject duplicate/unknown
keys, unsupported versions, malformed/truncated data and invalid types/bounds.
Record the operation identity, selected catalog path, recovery-set identity,
before/after digests and byte counts, ordinary mode, descriptive observed inode
identities, intended direction, last durable phase and terminal outcome.
Do not copy content, credentials, environment, runtime history or absolute paths.

Records are observations. File identities, digests, phase labels and identifiers
read after a restart cannot supply historical authority or recreate a handle.
The source inspector reports an explicit next action and every unavailable
authority. Empty legacy pending evidence remains an opaque blocker.

Prepare/read back/sync the initial record and its parent before active mutation.
Durably record a prepared candidate before either forward or reverse rename;
candidate metadata comes only from the actual retained prepared descriptor.
Record successful publication only after named-image readback and directory sync.
The phases are `prepared`, `forward_prepared`, `forward_published`,
`reverse_prepared`, `restored`, `aborted` and `forward_completed`.
Only the last three are terminal record outcomes; other outcomes are `pending`.
Use shared exclusive sibling staging for record replacement, with actual retained
candidate identity before any fallible rename/observation. Retain/reconcile an
uncertain record candidate before refreshing its owned metadata. A retry must
accept only the exact prior record or the actual owned prepared inode and bytes;
foreign edits, equal-byte replacement inodes and changed names remain conflicts.
No unchecked refresh may adopt another writer's journal.

## Checked terminal completion

Finish requires a real owned after-image. Reconcile any retained candidates,
revalidate the selected file and saved input, then read back and sync the exact
owned file and parent. Finish never publishes replacement bytes again.

Restore retains the existing checked reverse/abort behavior. For a durable
handle, both directions must publish, read back and sync their terminal record
before removing the exact owned pending entry and syncing its parent. A terminal
record alongside pending is still incomplete and blocks fresh Go/Python admission.

Once a terminal direction is selected, retries stay in that direction; a failed
forward Finish cannot be silently converted into Restore, and a reverse completion
cannot be converted into Finish. Reuse a checked already-published terminal record
after later I/O failure rather than manufacturing another rename or receipt.
Only completion of all required durability/removal checks marks the live handle
finished. Repeated completed calls refuse without another mutation.
Qualify a valid owned completion phase before latching its direction, then latch
before the first terminal-record mutation. Invalid/unapplied Finish must not
prevent the existing checked reverse/abort path.

Record, readback, sync, pending-removal and close errors retain the existing
operational/refusal distinction and typed active-publication uncertainty. Close
releases all owned resources despite cancellation, never finishes or rolls back,
and preserves unresolved pending evidence. Saved sets, holds and runtime history
remain untouched. A post-unlink parent-sync error does not recreate pending;
the chosen file and terminal record were already durable before unlink.

Completed records remain inert beside their associated saved sets. This slice
creates no repeated-retry records or unbounded history. Associated-record cleanup,
safe slot reuse, holds and later-release pruning remain explicit lifecycle work;
no installed upgrade or release claims completion before that work qualifies.

## Independent qualification

A separate spec-writer owns new Ginkgo/Gomega files. First observe missing API
compile evidence, then intended runtime RED through a compiled client calling the
actual production API. Core scenarios cover real forward completion, durable
reverse completion, occupied/foreign record refusal and inert restart inspection.
Use actual filesystem identities, saved originals and effective Git exclusion.

Cover immutable caller bytes, handle aliases, missing/negated/tracked exclusion,
private permissions, bounded schema and namespace enumeration, old v1 inspection,
terminal direction/retry rules, later edits and equal-byte foreign inodes.
Qualify real writes/renames/sync/readbacks followed by injected failures, compound
resource-close errors and every preserved active/pending/record identity.

Real ready-then-SIGKILL child controls observe interruption before/after active
renames, terminal publication and pending removal. Distinguish process death from
power-loss simulation. Fresh Go/Python admission must refuse while any pending
entry survives. No test-only exported execution wrapper is shipped.

Run focused race checks, the canonical Go pack quality gates and Linux/macOS CI.
Apply workflows/review-diamond.md with independent verification before findings
count. Do not claim full source, installer or release readiness from focused tests.

## Measurement

Keep the existing 30-package denominator and merged earned value 18 + 1/9 after
PR #121. Before implementation split R2 into two equal deliverables: R2.1 above,
and R2.2 qualified interrupted recovery with a newly granted capability, known
after-image reference and compatible-state/pending-resolution contract.
Only qualified and merged R2.1 earns 1/18 of the existing package:
`(18 + 1/9 + 1/18) / 30 = 60.5556%`, reported 60.6%. R2 stays partial until
R2.2 qualifies; public R3 and all activation/retirement/retention/release gates
remain open. Work packages, PRs and effort are separate measures.

## Required record shape refinement

Before parser hardening, two independent actual-record controls observed an
empty replacement's required `after.bytes` field being omitted or replaced by
JSON null while inspection still classified the record as checked. Both valid
record baselines passed; preservation and inert-authority checks passed before
the malformed-record status assertions failed. This is injected source-test
evidence, not a released incident.

The closed record format requires every declared top-level and nested field.
Nullable identity fields may contain null only where their phase permits it;
their keys still must exist. Required scalar values cannot be null. Enforce
presence and type alongside duplicate/unknown-key, phase and bound validation.
Keep inspection inert and preserve malformed evidence without changing v1
saved-set formats or reconstructing a live capability.

## Record-count admission refinement

Before correction, two independent paired actual-record controls admitted record
64 from a 63-record baseline, but also admitted record 65 from a complete checked
64-record baseline. The first namespace validation consumed the directory cursor;
later admission reused that retained descriptor and treated its exhausted cursor
as an empty directory. Prior records and the active original remained preserved.
This is injected source-test evidence, not a released incident.

Admission must use a complete bounded enumeration independently of a retained
directory's cursor position. Preserve validated metadata pins and ordinary
revalidation; use the already-checked count or a separately qualified fresh
enumeration. Exactly 64 existing records refuse a new slot; 63 may admit record
64. Retaining a descriptor is not evidence that a later directory read starts at
the beginning. Keep the saved-set and record bounds separate.

## Inspection error precedence refinement

Before correction, an actual record read followed by reported EIO produced
operational status 1. Adding an actual mode change at that boundary invalidated
the retained observation and incorrectly reduced the result to refusal status
2. Preserved bytes, unchanged inode, resource closure and false authority checks
passed before the status assertion failed. A metadata-only invalidation still
correctly refused with status 2. These are injected source-test controls.

Late revalidation must discard unsafe observations without erasing an already
observed operational failure. The returned operation and report Status retain
status 1 precedence when I/O occurred; ordinary metadata-only conflicts retain
status 2. Keep completeness false, unsafe rows discarded and all authority false.

## Terminal-direction admission refinement

Before correction, independent security and verifier controls performed a real
original-file sync during an unapplied durable Restore, then reported EIO. With
reverse completion already selected and no target rename, a subsequent Apply
incorrectly published the replacement. The completed-abort control refused
Apply without mutation; explicit final restoration and resource release passed.
This is injected source-test evidence, not a released incident.

Once a valid durable terminal direction is selected, Apply must also refuse
new forward publication. Only explicit retry of that chosen completion may
proceed. Apply must check the live direction before preparing or publishing any
candidate. Preserve invalid/unapplied Finish's non-latching behavior and legacy
handles without a durable journal.

## Qualified root spelling and final metadata classification

An accepted installation root may be relative (including `.`) or an absolute
path with a trailing separator. Preserve the existing no-follow component walk
and rejection of `..` before converting the qualified root to a clean absolute
spelling for Git's absolute repository identity and journal access. Bind that
spelling to the already-qualified installation; normalization must not admit
links or traversal, change the selected installation, or leave pending solely
because Git reports an equivalent absolute spelling.

Final recovery/publication revalidation must retain the failure classification
of both descriptor and pathname metadata observations. An operational error
first encountered in final revalidation remains `assessment_error`, status 1;
an observed identity, ownership or mode mismatch remains refusal status 2.
Missing expected objects and newly occupied missing entries remain conflicts,
while an operational failure observing a previously missing entry remains 1.
Do not collapse all failed metadata observations into a boolean conflict.

The shared recovery inventory must also preserve earlier operational failure
when final metadata invalidation discards its observations. Root status remains
`assessment_error` when any relevant I/O failed; clear invalid rows and totals,
set completeness false and keep every authority field false. Apply the same
precedence to early-return and final-return validation. Continue resource closure
without changing saved sets, publication records, installed files or processes.

Private metadata fault collaborators may qualify these boundaries by performing
the actual stat operation and then reporting a one-shot error. The production
default still performs native metadata observations; fault evidence must state
that the reported error was injected rather than a naturally occurring EIO.
