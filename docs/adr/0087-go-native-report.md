# ADR-0087: Native Go cost report

Status: implemented; focused local qualification complete, platform CI pending.

Decision: 86

## Context

After native init and doctor, the requested next command is `report`.
The existing report distinguishes deterministic facts, an explicit estimate,
and measured usage owned by the harness. Preserve that distinction and the
ordinary output contract from commit `9a3f2e4c61bbc92e4b05fde517cd60e78c4b7165`.
This closes only the report half of the existing reports/metrics source package.

## Decision

Route source-built `factory-go report` to Go without the legacy report script
or a sourced configuration library. Reuse the shared configuration grammar,
export precedence and process supervision; do not introduce another parser.
No model request, network access or dependency installation belongs to a report.

The invocation/template directory determines `scripts/hooks` and the default
`.factory/events.log`. A nonempty `FACTORY_EVENT_LOG` overrides the latter;
relative overrides resolve from the caller's working directory. Configuration
retains caller/Git discovery, explicit `FACTORY_CONFIG`, lexical legacy sibling
selection, and caller environment precedence including explicitly empty values.
Configuration values are data and must never be evaluated as programs.
Missing YAML and legacy files are permitted; environment/default values remain
available. `FACTORY_REVIEW_TOKENS` is environment-only, not a new config key.

Only a first argument exactly equal to `--clear` selects clearing. Remaining
arguments are ignored, as are all arguments in ordinary report mode. Clearing
is a best-effort unlink, before configuration or gate inspection: remove a final
symlink itself, never its target; preserve directories, including empty ones.
Suppress removal errors and retain the existing success line and zero status,
except for output failure or cancellation. Do not create directories or backups.

Count immediate hook entries ending in `.sh`, including directories, symlinks
and nonexecutable files, without recursion. A symlink or regular file at the
hooks-directory path has no traversed entries, matching `find` without `-L`;
missing or unreadable directory input is an error. Count nonempty event records,
including an unterminated final record. Render newline-terminated TSV records
using Bash's tab-IFS field behavior; omit rows without a nonempty second field.
Keep the existing facts, labeled estimate, usage pointer and no-savings disclaimer.
Whitespace-only records are nonempty and count; only zero-length records are
blank here. Count each hook entry once even if its filename contains a newline,
correcting the accidental overcount from the shell's `find | wc -l` framing.

Correct two accidental shell arithmetic behaviors explicitly: an empty/blank
log produces a single zero without an integer diagnostic; digit-only review
estimates are decimal, including leading zeros. Empty or nondigit estimates use
the existing default 3000. Parse nonnegative estimates as uint64 and check the
block-count multiplication; an out-of-range estimate or product returns a clear
nonzero diagnostic rather than wrapping, interpreting octal or inventing spend.
Preserve the supplied digit string when displaying the per-review factor;
normalization affects arithmetic, not the quoted estimate input.

Bound each configuration/event input at 16 MiB and the hook enumeration at
4096 entries. Check cancellation, reject unsafe special-file inputs, and report
read/bound failures with nonzero status. Missing event logs mean zero records.
Reject NUL-bearing event logs as unsafe binary input instead of reproducing
interpreter-dependent Bash field truncation. Configuration retains its existing
shared parser contract; this event-format restriction does not change that parser.
Regular-file symlinks may be read for compatibility; clearing never follows the
final symlink. Preserve broken-output errors and bounded Git discovery. Normal
reporting performs no project writes, including temporary backup artifacts.

## Qualification and boundaries

An independent evaluator writes native-route RED tests before production.
Compare ordinary output and statuses against the frozen shell implementation;
exercise config precedence, inert values, distinct caller/template roots, TSV
edge cases, hook types, clear preservation, decimal/overflow decisions, missing
and unsafe inputs, resource limits and output failure. Run the configured Go
quality gate and independent correctness/security review before merging.

This is source-native reporting only. Installed runtime activation, legacy
script retirement, recovery creation and retention remain separate milestones.
The shell implementation stays available until the migration cutover is qualified.
