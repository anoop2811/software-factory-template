# ADR 0094: live publication ownership and checked restoration

- Status: accepted source implementation refinement; no public activation consumer
- Date: 2026-10-04 UTC
- Decision: 93

## Purpose

Implement a real reversible publication component after merged PR #120. Current
v1 recovery sets contain checked reference bytes, not an applied forward action.
An absent path or imported receipt must not manufacture restoration authority.
This component derives reverse ownership from its own live publication and keeps
that evidence private. It refines specs/001-go-runtime-conversion.md:168 and
specs/001-go-runtime-conversion.md:321 without enabling the blocked public
restoration planner described by ADR 0092.

## Source boundary and caller obligations

Add an opaque live publication handle in the existing assessment package, reusing
its reference catalog, consent observations, pinned recovery reader and writable
storage primitives. External acceptance may call the production API directly;
do not add a shipping test driver, private bridge command or public restore flag.

The initial request names one existing catalog path, one existing integrity-checked
v1 recovery set, fresh explicit adoption confirmation for that single current
reference, and bounded replacement bytes. Reject unknown paths, absent/customized
originals, unsafe roots, invalid identifiers/confirmation and oversized data.
Copy caller-owned replacement bytes so later mutation cannot alter the operation.
Existing checked held sets may supply inert reference bytes; their hold is unchanged.
The backup must include the requested path and match the captured original. Re-sync
and read back the saved original and relevant recovery directories before mutation.
Do not rewrite the saved manifest, hold, payload or Git ignore/index state.

The existing set supplies only the before-image. Fresh consent qualifies the
current selected original; actual publication supplies the live after-identity.
No manifest, target revision, environment value, trusted boolean or serialized
handle supplies reverse ownership. There is no handle reconstruction/import API.

The future trusted installation consumer remains responsible for target
authentication or explicit local-source qualification, dependency closure,
semantic ownership, active/uncertain accounting and checkpoint assessment,
current-state compatibility, explicit unbridged-process quiescence and mandatory
activation checks. This source component establishes none of those prerequisites.
It cannot commit a forward installation or advertise rollback/activation readiness.

## Lifetime and exclusion

Acquire and retain the existing permanent exclusive transition guard for the
entire live operation, including restoration and checked closure. Revalidate the
physical root, controlled ancestry, saved input and selected leaf identities.
Initial support replaces existing ordinary single-link catalog files only;
creation, deletion, batch activation and absence ownership remain pending.

After qualification and before any active replacement, exclusively create the
empty 0600 regular `.factory/runtime-publication.pending` file. Pin its identity,
sync it and its parent, then recheck it. Never replace a pre-existing entry or
infer completion from its contents, age, PID, missing history or an unlocked lock.

Both Go and legacy Python shared and fresh exclusive guard acquisition must
refuse on ANY entry at that fixed pending path, including a link, directory,
malformed file, unreadable evidence or failed inspection. Perform the check while
holding the permanent lock and before owned execution work; recheck before return.
The live owner creates pending only after exclusive acquisition and retains that
same guard. Read-only, disabled and initially blocked commands remain inert.
This is a restart barrier for cooperating budget/loop execution, not proof that
all historical enforcement dispatch or unbridged processes participate.

## Publication and ownership

Prepare an exclusive sibling using the shared file-publication component.
Preserve the catalog's original complete mode for this first slice. Sync and
read back replacement bytes before publication; keep size at the existing 1 MiB
asset bound. Immediately revalidate the before-image and pinned parent before
renaming. Reuse shared preparation rather than duplicating its temporary-file
ownership rules. Extend that component only as needed to obtain its actual
prepared/published identity; caller-supplied expected metadata is not that proof.

After rename, retain the actual prepared inode plus immutable bytes and complete
mode as live ownership evidence. Read back the named after-image and sync its
directory. A publication error after rename may have changed active state and
must retain that typed uncertainty and pending evidence, including when cleanup
also fails. Repeated apply refuses rather than performing a second publication.
Do not silently restore or commit on an apply error.

## Checked restoration and closure

An explicit restore method accepts only this live handle under its retained
exclusive guard. Before any reversal, the named leaf must match the owned
after-inode, complete mode and bytes. A later edit, chmod, replacement, deletion,
link, unsafe ancestor or changed backup is a preserved conflict. Equal bytes in
a different inode are not ownership. Verify before preparing and again immediately
before publishing the saved original; do not touch unselected paths or history.

Restore the captured original bytes and original complete mode through shared
exclusive staging, read back the actual restored identity and sync its directory.
A restore interrupted after rename keeps its actual restored identity in the
live handle; a retry may complete durability only while that same before-image
remains unchanged. Never reconstruct this state from a receipt or absence.
A prepared handle may abort through the same checked method only if no active
mutation occurred and its original is still unchanged.

Only a checked durable return to the before-state qualifies removal of the
owner's exact pending entry. Recheck descriptor/path identity, unlink only that
entry, and sync its parent. A post-unlink sync failure reports uncertainty without
creating replacement evidence; the active original was durably restored first.
Preserve foreign pending entries and report removal/close failures safely.

Close releases every owned handle and the exclusive guard but never performs an
implicit rollback or clears unresolved pending evidence. Closing an unfinished
operation reports incomplete publication. Joining cleanup errors must preserve
the primary typed publication uncertainty. A crash loses the live capability;
pending evidence blocks new cooperating work and needs future qualified recovery.
There is no automatic marker deletion, forced restore, forward finish or retention.

## Qualification

Use outside-in Ginkgo/Gomega against the actual production API and filesystem.
Observe core RED before implementation: actual replace/restore, edited-after-write
preservation, and post-close admission refusal with no model/check execution.
Use real process readiness/termination handshakes for abrupt interruption; the
test owns final cleanup. A separate spec-writer owns all behavioral tests.

Cover fresh consent, selected/subset/held integrity-checked backups, unchanged
bytes/modes, caller slice copying, strict input bounds, symlinks/hard links/FIFOs,
root/parent/leaf replacement, repeated apply/restore, cancelled phases, real sync,
readback/close faults, and compound typed-error preservation. Verify actual
published inode ownership rather than mocked success. Requalify existing v1
inspection, creation, planning and shared Go/Python guards after reuse changes.
Run focused race tests, the existing Go pack quality gates and Linux/macOS CI.
Apply the review diamond with independent correctness and security verification.
No new dependency, pinned tool version, model call, network call or background job.

## Measurement and limits

Keep the fixed 30-package source plan and its earned 18/30 = 60.0% at this start.
The existing final third of the backup/rollback package has three equal source
deliverables: live transaction-owned restoration; verified durable forward/reverse
recovery lifecycle; public controlled rollback with compatibility integration.
This component earns only the first of those three after its actual qualification
and merge: (18 + 1/9)/30 = 60.3704%, reported 60.4%. The complete restoration
milestone and installed release acceptance remain open until the other two pass.
PR #119 planning and PR #120 transition exclusion earn no retroactive credit.

Complete v0.1.6 inventory/backup, transactional activation, legacy retirement,
later-release retention, packaged target qualification and the adopter pilot
remain separate required work. Existing public plans retain every false authority
field. The installed shell dispatcher remains unchanged.

## Pre-implementation qualification clarification

The component inherits the trusted, quiescent filesystem namespace boundary in
docs/adr/0091-durable-local-recovery-creation.md:96. Pinned identities, byte checks
and immediate pre-rename revalidation are not atomic compare-and-swap against a
hostile same-user writer. A caller must not present ordinary namespace checks as
global quiescence or protection against an adversary restoring every observation.

The missing new API may first produce a compile-time RED. An interface-only
unsupported stub may then make the independent fixture executable, so actual
replacement/restoration controls must also fail at runtime before behavioral
implementation. Record those phases separately; a missing symbol does not prove
an observed filesystem rollback defect.

Begin, apply, restore and close are filesystem-only: no Git query, native probe,
subprocess launch, script execution or model call occurs inside the component.
Reuse pinned recovery inspection and filesystem sync/readback; do not call the
Git-dependent recovery-creation orchestration. Acceptance fixture setup may use
the existing creation API before the component begins; clear process sentinels
after setup to independently prove the engine launches no process.

## Pre-correction cleanup refinement

An independent paired actual-descriptor regression observed that an unsafe
prepared-file mode plus a real close reporting EIO returned refusal status 2,
discarding the operational close failure. Preserve the safe refusal and report
the checked close failure as operational status 1, without printing private
paths or bytes. A confirmed close of the same unsafe descriptor remains a
refusal. Qualify both through the actual shared preparer and real file handles;
do not substitute an expected inode or export a test-only production wrapper.

A separate paired regression observed prepared abort without active rename:
the original remained durable and pending was actually unlinked, but a parent
sync failure made Close claim that pending evidence was retained. Use a neutral
incomplete-operation diagnostic requiring local inspection; do not assert
marker existence from the handle's old fields. The applied-operation control
already hid that wording through the typed publication error and did not
reproduce the direct-message defect. Preserve the same durability and retry
rules; this correction grants no cleanup or recovery authority.

## Pre-correction live handle alias refinement

An independent external API regression copied a successful prepared public
handle before any lifecycle method used its mutex. The original handle applied
the real replacement; the copied handle then reported successful restore and
removed pending while the named replacement remained. A copied phase snapshot
must not turn a live publication into a prepared abort.

Make the exported handle a wrapper around one private shared lifecycle state.
Every legitimate value alias shares phase, mutex, immutable images, pinned
ownership and resource closure. Restoration through an alias uses the same
actual after-inode; closing any alias closes that one operation. Nil and
zero-state handles refuse. There is still no import/reconstruction authority.
Retain all checked inode, byte, mode, ancestry and pending durability controls;
documenting a no-copy convention is not the ownership enforcement.

## Pre-correction post-rename observation refinement

Independent evaluator controls performed real forward and reverse renames and
successful named-stat calls, then reported a one-time EIO without changing the
filesystem. A target-stat or parent-refresh observation failure left stale pins;
once the fault cleared, explicit Restore still rejected the unchanged owned file.
All four controls failed before correction. This is injected I/O evidence on the
source component, not a released incident or an observed spontaneous OS error.

Retain the actual prepared candidate, immutable image and publication direction
before fallible post-rename observations. That candidate is not yet authority to
restore. A retry must reconcile the named target against its retained actual
descriptor, complete mode and bytes under the same exclusive guard, qualified
ancestry, saved input and pending ownership before accepting publication state.
Do not require stale old-target or vanished temporary-name observations to pass
before that reconciliation can run. Refresh only the qualified parent of this
operation's own mutation; never adopt an unexplained ancestor or foreign inode.

If a reverse rename actually published the original, a later retry finishes only
its checked durability and pending removal, without a third rename. If no active
rename occurred, abort still requires the original and prepared ownership to
remain unchanged. Edits, different equal-byte inodes, deletion, links, changed
ancestry or saved inputs remain preserved conflicts. Observation errors retain
typed uncertainty, resources and pending evidence; no automatic retry, imported
receipt, force cleanup or reconstructed ownership is authorized.

Use private per-operation observation collaborators with actual filesystem
defaults, reusing existing boundaries where appropriate. Qualify both directions
and target/parent observations with real rename and descriptor handshakes, plus
conflict controls after the observation failure. A successful synthetic callback
or merely setting the expected phase is not evidence of publication ownership.

Before the observation-status correction, expanded independent controls completed
all actual preservation, retry, descriptor-close and flock-release checks but
found four classification mismatches. Repeated parent-observation EIO reported
refusal status 2 instead of operational status 1 in both directions. Actual target
deletion reported operational status 1 instead of conflict status 2 in both
directions. Keep the existing status contract in
docs/adr/0091-durable-local-recovery-creation.md:56: ordinary observation I/O
failures are operational; missing or unsafe target observations and changed
identity, mode or ownership are conflicts. Reuse the existing assessment error
classification rather than duplicating errno policy. Preserve the same typed
uncertainty, private diagnostics, candidate, pending evidence and mutation limits.

## Pre-correction retained storage observation refinement

The latest advisory review identified the same classification inconsistency in
the retained writer's ordinary checks. Two independent controls performed a
successful actual named stat and then reported one-time EIO: immediately after
candidate promotion and during explicit restore after successful apply. Both
preserved the actual named inode, bytes and pending entry; later restoration and
exact descriptor/flock release succeeded before status assertions failed with
refusal 2 instead of operational 1.

Separate descriptor and named-observation errors from metadata mismatches in
every retained writer check, including root and ancestor observations. Reuse
the existing assessment error classification: ordinary I/O is operational 1,
while missing or unsafe names and actual identity, mode or ownership changes
remain refusal 2. Keep validation order, cancellation, safe diagnostics and all
ownership, mutation, durability and retry requirements unchanged. This corrects
the existing status contract; it grants no new publication or recovery authority.

## Pre-correction closed pending observation refinement

An independent control qualified actual original-file and parent sync, proved
the pending descriptor closed, and performed a successful named pending stat
before reporting one-time EIO. The same pending entry and durable original were
preserved; explicit retry removed the owned entry without another rename and
released every descriptor and the flock before the status assertion failed
with refusal 2 instead of operational 1.

Apply the same observation classification when pending is checked by its name
after its descriptor closes. Ordinary I/O remains operational 1; missing or
unsafe names and actual pending identity or metadata changes remain refusal 2.
Retain the original durability requirement, exact pending ownership check and
explicit retry rules. No new cleanup, reconstruction or recovery authority is
granted by this error-classification correction.
