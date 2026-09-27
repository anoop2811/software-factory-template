# ADR-0083: Inspect local recovery sets without mutation

Status: Accepted for source implementation; installed migration remains pending.
Date: 2026-09-27
Decision: 82

## Purpose and boundary

Implement the read-only integrity inventory prerequisite of backup/rollback in
specs/001-go-runtime-conversion.md:214 and specs/001-go-runtime-conversion.md:229.
Reuse the confined descriptor observer in assessment; do not call staging or
statefile operations that create directories or locks. No new dependency/pin.
The existing six-path preview remains partial, blocked and read-only.

## Public contract

Add one optional bare --inspect-backups to upgrade --dry-run --source PATH.
Attached booleans and duplicates refuse before any filesystem access. Without
--dry-run, preserve legacy dispatch. Without --inspect-backups, retain the exact
existing eight-field JSON schema and text, and do not open backup directories.
With the flag, retain schema_version 1 and existing fields, set
recovery_assessment to inspected, and add recovery_inventory with the contract
below. Text renders all inventory rows, reasons, counts and next actions; quote
untrusted relative names to prevent terminal control injection. No absolute paths
or content appear. Output errors keep status 1 through existing checked output.
Plan blockers, rollback_ready=false and applicable=false remain unchanged.
Inventory I/O errors take status 1 precedence; otherwise plan status still applies.
No Git/config invocation, downloads, models, locks, writes, chmod or unlink.

## Reserved record format

Inspection recognizes only a strict bounded UTF-8 JSON manifest.json in each
migration directory. This defines a future writer format, not a claim that a
trusted writer/transaction currently exists. Reject duplicate/unknown/missing
fields, trailing JSON, null values and wrong types at every object level.

Exact manifest fields:
- schema_version: integer 1
- migration_id: matches its directory; 1..64 ASCII letters/digits/hyphen/underscore,
  starting with an ASCII letter/digit
- source_revision and target_revision: 40 lowercase hexadecimal characters,
  distinct; source_revision equals the compiled six-file reference revision
- scope: g2-budget-loop-six
- held: boolean, descriptive only
- assets: nonempty array of at most six distinct catalog paths, lexically sorted

Each asset has exactly path, sha256, bytes, mode. These must match the compiled
reference catalog for that path (original active mode and size/digest). This
initial format supports only unchanged known legacy originals. No arbitrary
manifest path is opened and no metadata value authorizes restore/delete.
Original file copies reside at files/<catalog path>. Stored regular files,
including manifest, must have complete permission bits exactly 0600, nlink=1,
and owner UID equal to the current effective UID. Traversed .factory/backups,
set and nested directories must be owned by that UID with complete mode 0700;
.factory may have ordinary mode but must be an owned real directory without
special bits or group/other write. All paths are opened without following links.
Manifest limit 16 KiB; leaf file limit is the existing assessment 1 MiB limit.
Unknown/extra entries or missing files make the set incomplete. The only allowed
set entries are manifest.json and files; beneath files only required catalog
ancestors and the exact declared leaves may exist. Never read payloads of unknown
entries, links, devices or FIFOs. Empty extra directories also prevent integrity.

## Inventory result

recovery_inventory is an object with exact fields:
schema_version=1, root_status, complete, sets, set_count, file_count, bytes,
restorable=false, prune_authorized=false.
root_status is absent, inspected, unsafe, assessment_error or limit_exceeded.
Absent .factory or backups yields complete=true and empty arrays/counts. Present
safe roots with <=64 immediate entries are inspected, complete=true even when
individual rows are invalid (complete means enumeration, not recovery validity).
Reject unsafe roots without traversing them. Root I/O, mutation or enumeration
limits yield complete=false; never claim an absent/complete-empty result on error.

Rows are sorted by raw name, each with exactly path (relative
.factory/backups/<name>), classification, reason, next_action, file_count, bytes,
held (boolean or null), restorable=false, prune_authorized=false.
classification is integrity_checked, unrecognized, incomplete, unsafe,
assessment_error or limit_exceeded. Integrity_checked means only the recognized
manifest and inert copies match the compiled reference and contain no extras.
It does not prove provenance, transaction completion, current compatibility,
rollback eligibility or forward-release retention. Reason must explicitly say
transaction_authority_not_established for integrity_checked. next_action is
review_transaction_evidence for integrity_checked, inspect_preserved_set for
unrecognized/incomplete, resolve_unsafe_path for unsafe, retry_inspection for
assessment_error, reduce_inventory_scope for limit_exceeded.
Unknown directory names/manifests are unrecognized. Invalid JSON/schema/catalog
metadata is unrecognized. Missing required copies, different bytes or extra safe
entries are incomplete. Unsafe types, links, modes/owners or identity changes
are unsafe; OS read failures are assessment_error. Counts include only completely
integrity_checked sets' declared saved files and bytes; invalid rows have zero
counts. Manifest bytes are excluded. held is returned only after a structurally
valid manifest has been parsed; it is never authority. No count claims physical
storage usage for malformed/unknown trees.

Bound total traversal per set to 32 entries and depth 4 below its directory;
read directory entries with a bounded API, not unbounded ReadDir(-1). At root >64
entries, return root limit_exceeded and no rows (no partial arbitrary subset).
At set limits, preserve/report that set as limit_exceeded. Context cancellation
checks occur before opens, enumerations and reads; descriptors always close.
Pin root and directories; check each named identity/type/mode/owner again after
inspection and all opened regular file identities, complete modes, sizes and
mtime/ctime after reading. A changed root yields root unsafe, complete=false and
no rows. A changed set invalidates that set's success/counts. Listing changes
must be detected by directory identity and mtime/ctime checks. No cross-file
atomic snapshot is claimed; all metadata is advisory and later mutation must
revalidate under exclusion. Ordinary regular-file I/O is not promised interruptible.

## Qualification and progress

Independent outside-in Ginkgo/Gomega RED before production. Compiled tests must
cover absent/valid/held/multiple sets, deterministic text/JSON, exact default
compatibility, malformed/duplicate/unknown manifests, wrong revision/catalog,
customized/missing/extra copies, inert modes, hard/symbolic links, FIFO refusal,
root containment, limits, privacy and filesystem nonmutation. Internal controlled
fault/race tests must cover cancellation, unreadable input, root/set/leaf changes,
and descriptor closure. Use existing observer helpers rather than duplicate
low-level traversal. Qualify Linux/macOS and full source gates; no live model.

The existing G4 backup/rollback package has three equal source milestones:
read-only recovery integrity inventory; durable ignored recovery creation;
controlled installation-only restoration. This slice closes only the first
milestone after qualification and merge. The overall denominator remains 30:
(13 + 2/3 + 1 + 1/3)/30 = 50%. Transactional activation, complete installation
coverage, exclusion, backup creation and retention/pruning remain unfinished.

## Inspection clarifications

set_count always equals len(sets), including preserved invalid rows; file_count
and bytes sum only integrity_checked rows. Missing manifest is unrecognized.
held stays null until the entire manifest (including revision/catalog constraints)
is valid. Unknown extra directories are classified from their own no-follow
metadata and never descended into; safe extras make the set incomplete, unsafe
extras make it unsafe. Inspection does not claim to measure their nested storage.

Fixed row reasons by classification: unrecognized_recovery_record,
recovery_content_incomplete, unsafe_recovery_path, cannot_inspect_recovery,
recovery_limit_exceeded, and transaction_authority_not_established respectively
for unrecognized, incomplete, unsafe, assessment_error, limit_exceeded and
integrity_checked. Inventory-only status is 1 for any assessment_error, 2 for any
other invalid root/row, 0 for absence or fully integrity_checked rows. Public
preview combines with its plan status, with 1 preceding 2 preceding 0.

A manifest larger than 16 KiB is limit_exceeded without reading its contents.
A saved copy larger than the existing 1 MiB ordinary-file bound is unsafe,
consistent with the shared observation constraint. No partial count is retained.

Human-readable rows explicitly show held: true, held: false, or held: unknown
for a null marker, so held and unheld sets remain distinguishable in both formats.

Rows sort by raw directory-name bytes. The path field is a display identifier:
append the name to the fixed .factory/backups/ prefix after percent-encoding each
raw byte outside ASCII letters, digits, hyphen and underscore with uppercase
hexadecimal. Escape percent itself (and dots), preventing collisions with literal
escape-looking names. Valid migration IDs stay unchanged. JSON and quoted text
use the same display identifier; it is not a literal filesystem path to paste
into a command. This preserves distinct invalid-UTF-8 names without terminal
controls or lossy Unicode replacement. No decoded name is used for filesystem
access; the descriptor walker always uses the original name.

For root-chain revalidation, check identity, type, complete mode and owner on
all ancestors; compare listing metadata for the installation root and inspected
recovery directories. Unrelated sibling changes in shared ancestors such as
/tmp do not invalidate the inventory. Retain no-follow named metadata for safe
unknown extras and recheck it before reporting the set, without opening their
payloads or descending into them.
