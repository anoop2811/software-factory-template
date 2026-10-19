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
selection, apply, rollback or recovery flags. Completed controls and incomplete
cleanup emit a single JSON report on stdout without requiring `--json`. Invalid
arguments and preflight refusals use stderr without emitting a report.

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
command and accumulation of A and B recovery after C. Both initial criteria subsequently passed. PR #132 at `011d47b` passed all
18 hosted checks, including complete source gates on Linux/macOS and all four
native bundle targets. Its completed advisory model review covered 28/28 files,
39/39 fragments and 4/4 batches. Coverage is not approval; independent verification
confirmed one minor I/O classification issue, corrected in the follow-up commit.

Observed commands and output:

- `go test -race -count=1 -timeout=90s -v ./internal/installation ./internal/installationfs -ginkgo.no-color -ginkgo.fail-on-empty`:
  installation `28 Passed | 0 Failed`; installationfs `10 Passed | 0 Failed`.
- `./scripts/selftest/run.sh`: `217 passed, 0 failed, 0 skipped`.
- Hosted `make go-runtime-source-check GO_RUNTIME_TEST_TIMEOUT=25m GO_RUNTIME_TEST_GROUP=release-handoff GO_RUNTIME_TEST_PROCS=auto`:
  `44 Passed | 0 Failed` on Linux and macOS, including authenticated rollback to B
  after A removal and both internally coherent forged-alternate refusals.
- The complete discovered census is `2992 = 2851 + 97 + 44`. Existing platform-specific
  base-suite skips remain distinct from selected criteria; no gate was removed.

The follow-up error probe uses an external compiler overlay, not a shipped fault
flag: after real completed pruning, an injected `syscall.EIO` from either the backup
stat or journal observation originally returned status 2 and lost its cause.
The corrected code returns status 1 with `errors.Is(err, syscall.EIO) == true`.
The same two-case race probe passed: `2 Passed | 0 Failed`.

A local run that overlapped all three source groups encountered one installed-proof
timeout. It is not counted as a local full-suite pass. Hosted suites ran on separate
runners and passed; an isolated local hold/prune retry is recorded separately.

[Hosted source qualification](https://github.com/anoop2811/software-factory-template/actions/runs/39185206183)
and [advisory review](https://github.com/anoop2811/software-factory-template/pull/132#issuecomment-6257705743)
are tied to the head above. Final-head CI remains required before merge.

A subsequent review found two additional error-path defects. Recovery input custody
is now preserved before attempting diagnostic output, so an unwritable stderr
cannot discard inputs needed by a pending installation. A bounded external compiler
overlay with controlled acquisition/apply collaborators observed both legacy and
Go-predecessor cases fail before correction and pass afterward. It exercises the
real execution/finalization path and filesystem caches, not a new full authenticated
upgrade. Observed command `go test -race -overlay=/private/tmp/factory-pr132-diagnostic-yjcpm5rt/overlay.json ./internal/releasecmd -count=1 -v -ginkgo.no-color`:
`2 Passed | 0 Failed`. The overlay introduces no shipped fault switch.

Unexpected absence of a future directory without durable intent now returns
conflict status 2, while other I/O errors retain operational status 1. The new
independent real-filesystem regression first observed status 1, then passed with
status 2 while progress, journal and active files remained unchanged. Observed
command `go test -race -count=1 -timeout=90s -v ./internal/installation ./internal/installationfs -ginkgo.no-color -ginkgo.fail-on-empty`:
installation `29 Passed | 0 Failed`; installationfs `10 Passed | 0 Failed`.
