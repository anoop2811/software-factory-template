# Read-only recovery inventory

[ADR-0083](../adr/0083-go-recovery-set-inspection.md) defines the opt-in source
preview inspection. Installed migration, backup creation, rollback and pruning
are not implemented by this feature.

Run from the installation root with a locally built Go binary:

```sh
/path/to/source-built/factory upgrade --dry-run --source /physical/local/target --inspect-backups
/path/to/source-built/factory upgrade --dry-run --source /physical/local/target --inspect-backups --json
```

The preview lists immediate entries under `.factory/backups/`, their integrity
classification, preservation reason and next action. It does not create that
folder, modify `.gitignore`, execute saved scripts, write receipts or run Git.
Without the flag, the existing preview output stays unchanged and backups are
not inspected. The upgrade remains blocked: inspection never makes it applicable.

`integrity_checked` means a bounded recognized manifest and inert saved originals
match the compiled six-path legacy reference, with no unexpected entries. It does
not establish a completed migration, historical ownership, current compatibility
or authority to restore or delete. Do not hand-write manifests to grant authority.
There is no recovery-set writer yet. The format is a contract for future recovery
creation and current read-only validation, not an import mechanism.

An absent recovery directory produces an empty inventory. Invalid, incomplete,
unsafe, oversized or unreadable entries remain untouched and visible. Inspection
stops at unsafe paths and does not recurse into unknown extra directories. File
and byte totals count only declared saved files of integrity-checked sets, excluding
manifests; they are not total disk usage. Set count includes every reported row.
`complete` describes root enumeration, not whether any set can be restored.
A held marker is reported only from a fully recognized manifest and confers no
pruning or restoration authority.

Safety limits are explicit: 64 immediate entries, 32 visited entries per set,
16 KiB manifests, the six known reference paths, and private inert stored copies.
When inspection cannot finish safely, it reports that limit or failure rather
than calling the inventory empty. Both formats percent-escape unusual directory-name bytes in display paths;
these identifiers are not literal paths to paste into commands. Text also quotes
them; JSON preserves structured fields. No saved file contents or absolute installation roots are printed.

## What follows

Durable recovery creation must first establish effective Git ignoring, discovery
exclusions, transaction ownership and verified backups. Controlled restoration
must preserve subsequent edits and runtime history. Transactional activation and
successful-later-release retention remain separate prerequisites. The canonical
[conversion specification](../../specs/001-go-runtime-conversion.md) requires
eligible older sets to be pruned after a successful later forward release, with
holds, edits and incomplete cleanup explicitly reported. Inspection alone does
not implement or claim that lifecycle.

## Qualification

Independent compiled RED preceded production implementation:

```text
FACTORY_AGENT_ROLE=spec-writer go test ./acceptance -ginkgo.focus="G4 recovery inventory core" -ginkgo.no-color -count=1 -v
FAIL! -- 0 Passed | 3 Failed | 0 Pending | 1660 Skipped
FAIL github.com/anoop2811/software-factory-template/acceptance 2.023s
```

Independent follow-up RED cases exposed descriptor cleanup on cancellation,
held-state text omission, lossy filename display, and missing root/extra metadata
rechecks before their corrections. Final internal qualification ran:

```text
FACTORY_AGENT_ROLE=spec-writer go test -race ./internal/assessment ./internal/upgradecmd -ginkgo.no-color -count=1 -v
SUCCESS! -- 55 Passed | 0 Failed | 0 Pending | 0 Skipped
ok github.com/anoop2811/software-factory-template/internal/assessment 2.280s
SUCCESS! -- 8 Passed | 0 Failed | 0 Pending | 0 Skipped
ok github.com/anoop2811/software-factory-template/internal/upgradecmd 1.805s
```

Independent correctness and security reviews reported no remaining actionable
findings. Full compiled regression, source-gate and exact-head Linux/macOS CI
evidence belongs in the PR. Ownership-change filesystem cases require local
privileges; invalid-byte filename fixtures depend on filesystem support. A
filesystem-independent raw-byte serialization regression covers display identity
without that filesystem dependency. These results do not establish installed
activation, backup creation, restoration or retention/pruning.

## Discovery prerequisite

[Recovery discovery exclusions](RECOVERY_DISCOVERY.md) keep factory-owned gates
from consuming obsolete saved scripts, tests and documentation. This prerequisite
does not create recovery sets or establish universal exclusions for arbitrary
user commands and external scanners. Durable creation remains pending.
