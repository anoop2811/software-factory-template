# ADR-0081: Explicit adoption of unchanged legacy factory assets

Status: accepted for private source qualification; installed activation pending.
Decision: 80. Date: 2026-09-27 UTC.

## Decision and user-visible constraints

The user approved legacy adoption on 2026-09-27 UTC and required transparent
migration without accumulated obsolete files. Refine AC 3.4, AC 4.1 and FR-015
in specs/001-go-runtime-conversion.md:176: an explicitly authorized operator may
grant new management authority over selected unchanged known factory assets.
This is not proof of who installed them. Historical origin remains unproven.
This narrowly supersedes ADR-0080's prohibition on explicit adoption and its
requirement for positive historical origin as the only ownership mechanism.
Local metadata still cannot manufacture authority, and ordinary plan is unchanged.

Preview must show paths, reasons and remaining blockers. Preserve modified and
unknown content. Successful eventual migration removes superseded active logic;
retained public paths are thin adapters, not duplicate implementations. Verified
recovery is private and ignored under .factory/backups/MIGRATION_ID/. A successful
later distinct release prunes eligible older sets and retains the preceding
installation's recovery. Edited, held, unsafe or incomplete sets are visible
exceptions with counts, bytes, reasons and a next action, never silent success.
These existing lifecycle requirements are not implemented by this ownership step.

## Private commands and explicit authority

With FACTORY_BRIDGE_PROTOCOL=1, add two literal requests:

- `factory migration propose-adoption ROOT PATH...`
- `factory migration plan-adopted ROOT SOURCE DIGEST PATH...`

Require one to six unique exact catalog paths; reject aliases, unknown names,
duplicates and invalid arity before reading. Canonicalize selections to catalog
order. Reuse the six immutable references and confined observer from ADR-0079.
Selected files must all be safe single-link ordinary files matching reference
content and full mode. Missing, customized or unsafe selections return status 2;
I/O errors return 1. Refusals emit sanitized diagnostics and no authorization JSON.
Read-only operations create no files, receipts, caches, backups or locks, perform
no network/model/subprocess work, and leave public upgrade dispatch unchanged.

A proposal is a newline-terminated JSON object: schema_version=1,
reference_revision, scope=g2-budget-loop-six, purpose=adopt_known_legacy_assets,
prior_origin=unproven, ownership_authorized=false, proposal_digest, root_identity
and assets. root_identity has device and inode as decimal strings. Each asset has
path, reference {sha256,mode,bytes}, identity {device,inode,type,mode,bytes,
mtime_seconds,mtime_nanoseconds,ctime_seconds,ctime_nanoseconds} and observed
{sha256,mode,bytes}. Identity device/inode and timestamp seconds are decimal
strings; type=regular, full mode is four-digit octal, nanoseconds/bytes integers.
No absolute paths or raw content are emitted. Return 0 only for a valid proposal;
this means a proposal is available, not ownership, readiness or apply success.

The digest is lowercase SHA-256 over a domain-separated canonical serialization
of purpose, schema, scope, reference revision, physical root identity, canonical
selection and every listed asset field. Bind all fields explicitly using a typed
payload; test a golden vector and guard against omitted fields. Use a fixed domain
prefix `software-factory/legacy-adoption/v1\n` before compact JSON in declared
field order. The canonical payload keys, in order, are purpose, schema_version,
scope, reference_revision, prior_origin, ownership_authorized, root_identity,
selection (canonical path array), assets. Nested key order is the order listed
above: root device/inode; asset path/reference/identity/observed; observations
sha256/mode/bytes; identity device/inode/type/mode/bytes/mtime_seconds/
mtime_nanoseconds/ctime_seconds/ctime_nanoseconds. The prefix ends in one literal
LF byte. Use compact JSON with no trailing newline. Only proposal_digest is
excluded; constant report fields are bound too.
The digest is not authentication, a secret or proof that a human
entered the command. Authority comes from an explicitly authorized CLI caller.
Environment, configuration, version/ownership files and saved JSON never supply
confirmation. Changing root identity, selection, bytes, inode, type or complete
mode invalidates it. Reordered equivalent selections retain the same digest.

plan-adopted requires the exact digest as a positional operand: exactly 64
lowercase hexadecimal characters. Malformed or mismatched digests and changed
selected observations return conflict status 2 with sanitized stderr and no JSON.
I/O, cancellation, output and unexpected failures return 1; invalid/unsafe root
syntax retains the assessment status 2 contract. Reobserve the
selected installation while holding both roots pinned through planning. Compare
the current proposal to DIGEST, compare selected observations used in action
planning (including inode, times, full mode, type, digest and size), and reobserve
selections again before returning; reject changes and
unsafe roots with no partial authorization output. Reuse action planning and
containment, not a second filesystem implementation. Preserve cancellation and
output failure handling. These observations are not a filesystem-wide atomic
snapshot and do not establish migration exclusion.

The adopted output embeds the existing plan JSON fields and adds
ownership_basis=explicit_operator_adoption, proposal_digest and authorized_paths
in catalog order. prior_origin remains unproven. ownership_authorized means all
six catalog files were selected and matched; a partial selection reports only its
authorized_paths and ownership_authorized=false. Replace prior_origin_unproven
in blockers with ownership_scope_incomplete for a partial selection; remove only
that blocker for complete selection. Always retain the other four blockers,
source_authentication=unverified_local, activation_ready=false,
rollback_ready=false and applicable=false. Existing customized/unselected files
keep normal preservation/conflict actions. Return status 2 for a complete blocked
report, or 1 for per-file assessment errors. Explicit consent never qualifies the
target runtime, authenticates the source, establishes quiescence or proves recovery.

No persisted output grants future authority. Eventual apply must require explicit
confirmation and revalidate under migration exclusion before mutation; it must
also satisfy authentication, compatibility, recovery and retention prerequisites.
The public UX will compose confirmation with upgrade rather than require users
to manage internal receipts. This private protocol is source qualification only.

## Qualification and honest progress

Independent Ginkgo/Gomega compiled acceptance must demonstrate RED before
production. Cover valid full/partial selection, stable canonical digest, malformed
or mismatched digest, copied roots, replaced same-byte inode, content/mode edits,
unsafe links/types, absent/unknown/duplicate selections, poisoned environment and
local receipts, preserved customizations, no writes, literal operands and old
assess/plan compatibility. Controlled internal faults cover cancellation, roots
replaced while another tree is read, selected files changed during target reading,
I/O failures, output errors and descriptor closure. Qualify on Linux/macOS with
race tests, source gates and independent correctness/security review.

The approved authority alternative replaces the remaining historical-origin-only
half of ADR-0079's second milestone; it does not add a milestone. Only completed,
qualified, merged implementation moves (13 + 2/3 + 1/2)/30 = 47.2% to
(13 + 2/3 + 2/3)/30 = 47.8%. Public preview, activation, recovery, cleanup and
retention remain pending. This decision alone receives no completion credit.
