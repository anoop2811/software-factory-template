# ADR-0090: Native Go legacy configuration migration

Status: accepted; native source implementation; installed cutover separate.

Decision: 89

## Context

Review-lane is merged in PR #116. Complete the requested six-command source
conversion with `migrate-config`, using frozen commit
`c10739fa95ff2123946a43388153ae931b593804` for ordinary compatibility.
Decision 41 remains authoritative: parse legacy data, preserve configured YAML
values, retain the old file as `factory.config.migrated`, and leave an
uncommitted diff for the adopter to review. No automatic commit or deletion.

## Command and parsing contract

Native `migrate-config` accepts only repeated `--dry-run`; other operands return
status 2 with the existing diagnostic. Discover Git root through the shared
supervisor with a ten-second deadline, falling back to cwd for an ordinary Git
failure. Cancellation, output overflow or uncertain cleanup returns nonzero.
Use the selected project's `factory.yaml` and `factory.config`, without the old
migration script or sourced libraries. Require YAML first; absent legacy input
is a successful no-op. Status and ordinary stdout remain compatible.

The old script checked root YAML but let FACTORY_CONFIG redirect its setter to
another file. Correct that mismatch: when migration work exists, a nonempty
override must identify the same existing YAML inode as the root target, resolving
relative overrides from root. Otherwise refuse before mutation, including dry
run. Always publish the root YAML. Do not clean symlink-sensitive interior
components while deciding whether the override identifies that file.

Parse physical legacy lines, including an unterminated final line. Ignore empty
lines, first-column comments, lines without equals and keys outside nonempty
ASCII letters/digits/underscore. Preserve the baseline's leading-digit keys;
do not accept export prefixes or whitespace around keys. Lowercase ASCII keys.
An immediately quoted value ends at its first matching quote; ignore the tail.
Unquoted values strip a whitespace-before-hash comment and trailing ASCII
whitespace, preserving leading whitespace. Treat values as inert data.
This grammar intentionally differs from the runtime export parser.

An existing nonempty value from the shared flat YAML reader wins. Missing or
blank values receive the legacy value through the shared rewrite algorithm.
Preserve duplicate-key behavior: apply checks the evolving YAML after each
planned rewrite, while dry-run checks the unchanged original for every line.
Reuse exact setter physical-line behavior, including unterminated append rules.
Dry-run preserves all files and reports would-add/keep counts. Apply records
config_migrated yes and reports moved/kept counts before the recovery guidance.

Reject NUL inputs and candidate values containing CR, LF or double quotes that
cannot round-trip through the existing setter. Validate the entire plan before
mutating; a rejected later setting must not leave earlier settings applied.
Bound each input and generated YAML/output at 16 MiB and parsed settings at 4096.
Use checked output and cancellation throughout; never invoke a shell or model.

## Mutation and recovery contract

Operate in a trusted, quiescent project directory, pinned and checked for
observed replacement. Require caller-owned ordinary single-link input files;
refuse symlinks, hardlinks and special files. Apply requires writable YAML and
safe directory permissions; dry-run can inspect read-only ordinary YAML.
Refuse any existing `.migrated` destination, including directories and dangling
symlinks, before publishing YAML. Preserve existing recovery bytes and metadata.

Prepare the whole YAML result first. Reuse guarded configuration publication
through a narrow shared transform/rewrite API, keeping config.Set behavior
unchanged. Recheck original bytes/metadata and legacy identity before atomic
YAML publication; preserve YAML permissions/ownership and clean identified
temporary files. Do not publish each migrated key separately.

After YAML publication, recheck the project and legacy snapshot and rename the
legacy file to `.migrated` with an atomic no-replace primitive on Linux/macOS.
Do not fall back to a clobbering rename when no-replace is unsupported. An
unexpected destination must survive even if it appears after preflight.
Retain the original legacy inode and permissions under the recovery name.
This is not a two-file atomic transaction: if interruption or rename failure
occurs after YAML publication, retain the original legacy file, report the
partial state, and return nonzero. Do not erase the complete new YAML or an
unknown destination to disguise failure. Output failure after success also
returns nonzero without undoing the completed migration.

The one established `.migrated` artifact is retained for manual recovery, not
duplicated on reruns. A repeated successful migration is a no-op. This command
does not introduce numbered backups or implement runtime-recovery retention.

## Qualification and rollout

Independent Ginkgo/Gomega core RED precedes production. Compare ordinary apply,
dry-run, no-op, arguments, quotes/comments, duplicates, existing/blank values
and file bytes with the frozen script. Qualify override confinement, unsafe
files, bounds, literal values, output failure, signals, changed input and a
destination introduced at the publication boundary. Run existing config-set,
review-lane and CLI regressions after extracting the transform API. Complete
independent review and exact-head Linux/macOS CI before merge.

Keep the installed legacy script route. This closes source implementation of
the requested command list, not installed Go activation, script retirement or
the separately specified ignored runtime-recovery lifecycle.

Qualification clarifications: when legacy input is absent, an otherwise safe
read-only project remains a successful no-op; defer apply writability checks
until migration work exists. A post-publication input replacement can invalidate
the original pathname, so partial-state diagnostics must request inspection
without claiming that `factory.config` still names the original recovery bytes.
