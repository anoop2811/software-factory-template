# Native Go legacy configuration migration

Decision 89 and [ADR-0090](../adr/0090-go-native-config-migration.md) define the
native `migrate-config` command. It reads `factory.config` as data, keeps existing
nonempty YAML values, fills blank or missing values, and records
`config_migrated: "yes"`. It does not run shell expressions or call a model.

`factory migrate-config --dry-run` shows the proposed settings without changing
files. Repeated `--dry-run` flags are accepted. Unsupported operands return the
existing argument diagnostic. Commands invoked below a Git project select its
root; an ordinary Git discovery failure uses the invocation directory.

Apply prepares the complete YAML transformation before publishing it once. The
legacy file is then renamed to `factory.config.migrated` with a no-overwrite
operation. The rename preserves its inode and permissions. No commit is made;
review the uncommitted diff and commit it yourself. A successful rerun is a no-op
and creates no additional recovery files.

## Refusal and recovery

An existing `factory.config.migrated` is never overwritten, including a directory
or dangling symlink. Inspect that recovery artifact before deciding where to
move it. Input files must be caller-owned ordinary files with one hard link;
symlinks and special files are refused. Dry-run can read an ordinary read-only
YAML file, but apply requires writable configuration and a safe project directory.

When migration work exists, an explicit `FACTORY_CONFIG` must identify the same
existing YAML file as the project's `factory.yaml`. Migration always publishes
the root YAML. This corrects the old script's mismatch between the file it
checked and the file its setter could update.

Values that cannot round-trip through the existing flat setter are rejected
before mutation. NUL input, oversized files/results and excessive settings also
produce a nonzero result. The command preserves the migration-specific parsing
grammar; it does not reinterpret legacy configuration as shell code or adopt
the different runtime-export grammar.

YAML publication and the legacy rename are two separate operations. If a signal,
changed input or rename failure stops the command after YAML publication, the
complete new YAML remains, and the command does not remove the legacy input.
Observed external changes can mean its original pathname no longer identifies
the recovery bytes. The diagnostic reports the partial state and requests
inspection before retrying; an unknown recovery destination is preserved.
Output failure can also return nonzero after a completed migration.

## Rollout boundary

This command converts configuration data, not the installed factory runtime.
Installed script entry points remain routed as before. Go activation, retiring
redundant scripts, and the ignored runtime backup/retention lifecycle are separate
conversion work. The single established `.migrated` configuration recovery file
is retained under the existing compatibility contract.

## Qualification

Independent core acceptance first reported `0 Passed | 3 Failed`. Subsequent
review regressions exposed read-only no-op rejection and unsupported recovery
claims after observed legacy replacement/removal. Both failed before correction.
Omitting the final staging guard produced `0 Passed | 2 Failed`; restoring it
passed, establishing that the boundary tests detect its absence.

The final independent safety selection passed 34 cases with both the compiled
CLI and outer race detector (`17.366s`). Eight internal publication cases passed
under race detection (`1.436s`). The combined migration, configuration writes,
precedence, review-lane and Cobra regression selection passed (`178.917s`).
Pack lint reported `0 issues.`, vet exited zero, gosec reported `Issues : 0`,
govulncheck reported `No vulnerabilities found.`, and the shell selftest reported
`217 passed, 0 failed, 0 skipped`. Independent review found no remaining
actionable issues. Exact-head Linux/macOS CI is required before merge; its final
results and execution commands belong in the PR evidence.
