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
creation. Apply/Restore/Finish/Close remain filesystem-only. No command, check,
model, network request, background work or new dependency is introduced.

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
confirms it is effective. No matching manifest or caller boolean proves that.

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
