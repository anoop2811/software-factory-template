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
