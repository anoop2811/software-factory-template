# ADR 0106: retain one usable predecessor and prune qualified older backups

Date: 2026-10-19 UTC
Status: user-authorized implementation contract
Decision: 105

## Scope

Implement specs/001-go-runtime-conversion.md:218 (AC 4.5),
specs/001-go-runtime-conversion.md:225 (AC 4.6),
specs/001-go-runtime-conversion.md:338 (FR-033/034), and the recovery lifecycle
in specs/001-go-runtime-conversion.md:434. This closes automatic backup retention
and its hold/prune controls, not default Go selection, a release, the adopter
pilot or actual four-platform release authentication.

## Release advancement

A successful distinct forward official release B -> C may prune eligible recovery
for A only after C's deterministic checks, durable installed-version transaction,
new recovery for B and pending-marker cleanup succeed. Keep B independently
usable relative to C; restoration must never depend on a pruned A set. Initial
legacy -> B retains its predecessor. Only strictly increasing valid semantic
release versions advance automatic expiry; incomparable tags, same-version or
build-metadata-only changes, explicit rollback/recovery, local/unversioned images,
failed checks and pending transactions do not advance it. Skipped releases count
as one transition, not as elapsed time. Existing CLI and status semantics remain.

## Authority and compatibility

Backup/journal records and the new versioned retention sidecar contain descriptive
identity hints, project inputs and actual saved-file observations. They never
confer mutation, historical-check or restoration authority. Capture saved-copy
identities after backup, not the original active-file inode identities. Keep the
existing whole-installation manifest and generation recipe compatible. Older sets
without sufficient fresh proof remain visible preserved exceptions; a supported
legacy set may qualify from embedded pinned references and a fresh known target.

Reuse authenticated source-image inspection, canonical installation roles and
the closed generation recipe to qualify both ends of a recovery set. Go history
requires its freshly authenticated historical image; fixed legacy profiles use
the embedded pinned catalog. A narrow resolver reuses the existing fixed-repository
release acquisition and independent trusted verifier; it never executes saved
scripts, historical runtime binaries or models. Download/authentication failure
preserves the affected set and names the next action. No new external runtime,
service, secret or dependency version is introduced. Use the existing permissive
Go modules and standard library rather than a second verification/filesystem stack.

Automatic deletion is anchored to the live successful B -> C capability and a
fresh check of its private candidate proof/current C, plus a complete B recovery
qualification. Explicit/restarted pruning freshly qualifies current C and another
usable recovery set. It derives the full before/after role delta from verified
images, never a local journal's `retain` assertions or a count of completed records.
Changed B roles must have exact saved before-images; unchanged B roles must remain
available in freshly checked C. A forged empty/partial alternate grants nothing.

## Preview, controls and reports

Ordinary candidate and authenticated installation preview reports retention
choices without project writes. Bind observations and proposed deletions into
the confirmation digest; recompute under the existing exclusion lifetime before
mutation. A changed hold, set, journal or file requires a fresh preview.

`factory upgrade --keep-backup MIGRATION_ID` holds a checked complete set for
reference. It is idempotent, performs no source selection/activation/model call,
and cannot make an incomplete set restorable.
`factory upgrade --prune-backup MIGRATION_ID` requests eligible older deletion,
including a held set, after the same fresh ownership and alternate-recovery proofs.
These standalone modes cannot combine with apply, rollback, recovery or source
selection. Reject malformed/repeated/ambiguous operands before legacy dispatch;
use Cobra. Completed controls and incomplete cleanup emit one JSON report on stdout.
Invalid/preflight refusals use stderr without a report. Preserve exit 0 success,
2 invalid/ineligible/conflicting input and 1 operational/incomplete cleanup. Local images never auto-expire.

Reports identify every retained/removed set, reason and next action, with file and
byte totals. Holds, edits, unsupported/unknown records, links, foreign ownership,
unsafe paths, tracked or no-longer-ignored recovery and unavailable proofs remain
explicit exceptions. Healthy installation plus failed pruning is cleanup
incomplete, not failed activation or silent success. Runtime accounting, loops,
application/configuration files and customization remain untouched. Preserve pending recovery inputs before diagnostic output, even when that output fails.

## Confined durable deletion

Retain root/transition exclusion across proof and cleanup. Reuse installationfs
owner/type/link-count/identity/digest checks and filepublish's durable publication.
Validate the closed expected file/directory set; never recursively remove an
unclassified directory or follow links. Recheck every owned leaf and physical
parent immediately before unlink. Remove only checked empty directories in
postorder and sync each changed parent. Preserve anything changed since planning.

Keep a bounded prune-progress record under the already ignored backup root; reserve
its name explicitly in discovery/quota calculations. Persist intent before each
mutation and completion after its durability boundary. Interrupted sets are not
advertised as restorable. Resume only with fresh current/alternate/source proofs
and validated progress; a missing path without matching durable intent is conflict.
Replayed progress cannot authorize deleting a new/changed entry or the sole usable
recovery. Retire an associated old installation journal only by its derived ID,
validated terminal schema and pinned observation after its backup is gone. Keep
progress/tombstone metadata bounded rather than accumulating per-release remnants.

## Independent acceptance and review

The independent spec author must observe public RED before production changes,
then cover real A -> B -> C retention, initial/same-release/local/rollback/failure
negative controls, hold/explicit prune, sole usable/forged alternate refusal,
changed/unknown/unsafe/ignored/tracked entries, interrupted deletion/retry, counts,
bytes, idempotence, no model/runtime/compiler dependency, preview immutability and
changed-since-preview refusal. Use real compiled CLIs and real filesystem/lock
boundaries; collaborator tests supplement those proofs.

Run the relevant race suites and configured Go/repository gates. Preserve the
complete disjoint criterion census and all Linux/macOS/native-target CI checks.
Run the review-diamond graph, verify each finding and respond to model findings.
Protected production paths require human review; no automatic merge.

## Report shape

`RetentionProposal` embeds the unchanged installation proposal and adds
`retention`. Reports have schema_version 1; mode is retention-preview,
retention-apply, keep-backup or prune-backup; status is planned, completed or
incomplete; advanced records whether this invocation advances automatic expiry.
Each sets row has migration_id, path, action, reason, files, bytes, next_action
and restorable. Actions are retain, prune, removed, held, already-removed or
incomplete. These control reports do not themselves advertise restoration
authority: restorable is false. Files/bytes include all regular files inside
the backup directory, including metadata, and exclude the separately retired
transaction journal. Unsafe/unreadable totals are null. Aggregate removed_sets,
removed_files and removed_bytes count confirmed removals; retained_sets,
retained_files and retained_bytes sum known retained observations. counts_known
is false when any retained count is unknown.
