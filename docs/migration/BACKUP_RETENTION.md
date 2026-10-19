# Upgrade backup retention

The approved contract is [ADR 0106](../adr/0106-go-backup-retention.md).
This is source implementation for the explicit Go candidate flow; the default
cutover and replacement release still require the gates in
[GO_RELEASE_READINESS.md](GO_RELEASE_READINESS.md).

## Ordinary upgrades

Preview and confirm a later candidate using the existing
[public upgrade commands](GO_CANDIDATE_HANDOFF.md). JSON preview includes a
`retention` report; text preview includes the same report on a `factory retention:`
line. The confirmation digest covers existing backup observations and holds.
Changes after preview require a new preview.

After A -> B, retain recovery for A. After a successful authenticated B -> C,
qualify recovery for B against fresh source images before deleting eligible older
recovery for A. Keep the immediately preceding usable installation. Initial
migration, failed upgrades, explicit rollback/recovery, local images, same-release
runs and incomparable release tags never advance automatic expiry. Elapsed time
alone does not expire anything.

Backup sets live under the private, Git-ignored `.factory/backups/MIGRATION_ID/`.
Saved scripts are inert reference material; neither cleanup nor restoration
executes them. Runtime history, accounting, application files and customizations
are outside this cleanup. The old transaction journal is retired only after its
qualified backup is removed. One bounded progress record replaces a growing
collection of cleanup tombstones.

## Keep a backup for reference

From the project directory:

```sh
./factory upgrade --keep-backup MIGRATION_ID
```

This adds an idempotent protective hold to a complete checked set. It does not
activate anything, acquire releases or invoke a model. Automatic cleanup preserves
held sets. A hold cannot repair missing files or grant restoration authority.

## Request older-backup cleanup

```sh
./factory upgrade --prune-backup MIGRATION_ID
```

This also accepts a held older set, but only after freshly authenticating the
current and historical release images and independently proving a usable alternate
recovery. It refuses the current/sole immediately preceding recovery and any
unqualified selection. The controls are standalone; do not combine them with source
selection, apply, rollback or recovery flags. They emit a single JSON report on
stdout without requiring `--json`.

An unavailable historical release or verifier prevents deletion. No local receipt,
completion label or saved checksum substitutes for fresh source ownership proof.
Older supported sets without the new sidecar may qualify using their checked
manifest and independently obtained source images; otherwise they are preserved.

## Read the result

Reports show each retained or removed set, its reason, next action, and file/byte
counts. Counts include metadata inside the backup directory and exclude its separate
transaction journal. Unknown or unsafe counts are null and `counts_known` is false.
Removed totals count only removals confirmed by this invocation. These reports
always set `restorable` to false: a control report is not permission to restore.

Edited files, extra entries, links, unsafe ownership or modes, active transactions,
unrecognized metadata, tracked/unignored paths and missing proofs are preserved
conflicts. They require inspection; cleanup does not recursively delete unknown
content. Git ignore policy is checked against the repository's actual rules.

A successful apply emits its retention report on stderr, preserving existing
stdout. Exit 0 means completion; exit 2 means invalid, ineligible or conflicting
input; exit 1 means operational failure or incomplete cleanup. If activation
succeeded but cleanup failed, the diagnostic says the installation is healthy.
Preserve the progress record and reconcile the reported conflict before retrying
`--prune-backup` for that same migration ID. Retry authenticates and checks again;
it cannot delete a new or changed entry merely because an earlier intent exists.
A partial set is never advertised as usable recovery. A completed retry is
idempotent and does not recreate deleted backups.

## Qualification

The independent public criteria first observed RED for the missing installed hold
command and accumulation of A and B recovery after C. The focused green evidence,
complete source gates and review results will be recorded after they run.
