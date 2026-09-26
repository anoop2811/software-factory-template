# ADR-0079: Read-only Go installation reference assessment

Status: accepted for private source qualification; ownership and activation pending.
Decision: 78. Date: 2026-09-26 UTC.

## Purpose and measurement

Before installing the Go budget/loop runtime, observe which legacy files still
match a known reference without treating matching bytes as installation ownership.
This supplies one prerequisite for specs/001-go-runtime-conversion.md:176 and
specs/001-go-runtime-conversion.md:195. It is not the complete public upgrade
preview, a migration authorization, or permission to replace or remove files.

Keep the established 30-package denominator. Divide the existing G4 ownership/
preview package into three equal milestones: confined reference assessment;
trusted prior-origin and target-action planning; integrated public read-only
preview qualification. This slice earns only the first third when acceptance
passes. It earns no G2 activation/retirement credit and closes no G4 release gate.

## Private compiled boundary and reference

Under FACTORY_BRIDGE_PROTOCOL=1 accept exactly `factory migration assess ROOT`.
Use the existing Cobra private request registration; ROOT is one literal operand.
Do not add public upgrade routing, flags, source downloads, subprocesses, probes,
candidate execution, model calls, or persistent files. Capture cancellation at
the private operation boundary and release all owned handles on return.

The reviewed embedded reference is commit
c8f8d34edbc14df5655fcbf0aab7eea46ced0295, limited to these six sorted paths:

- scripts/factory-budget.sh
- scripts/factory-loop.sh
- scripts/lib/budget-config.sh
- scripts/lib/budget.py
- scripts/lib/budget_adapters.py
- scripts/lib/loop.py

Record each Git tree mode as Unix permission bits (0755 or 0644), SHA-256 of
literal blob bytes, and byte length in a generated Go catalog. Record the source
revision in that file and compare catalog observations against the immutable Git
blobs in acceptance. The catalog is compiled reviewed reference data, not a local
manifest. Do not read .factory-version, an adopter ownership manifest, arbitrary
target intent or docs/migration/baseline-assets.json as authority. Unknown extra
files are outside the six-path scope and must not be traversed.

## Output and status

Successful assessment emits one newline-terminated JSON object, with no raw file
contents or absolute project paths. Fields are schema_version (1),
reference_revision, scope (g2-budget-loop-six), ownership_authorized (false),
activation_ready (false), assets (six sorted rows), and counts (all five
classification keys, including zeros). Each row has path, classification,
reference {sha256,mode,bytes}, observed (same shape or null), and reason.
Modes are four-character octal strings; bytes are integer lengths.

Classifications and fixed reasons:

- matching_reference / content_type_mode_match: safe regular single-link file,
  same complete bytes, size and mode as reference; observed is populated.
- customized / content_or_mode_differs: safe bounded ordinary file differs in
  bytes or permission/special mode bits; observed is populated (special bits
  force customization, even if the displayed permission bits match).
- missing / path_absent: absent leaf or ordinary missing ancestor; observed null.
- unsafe / unsafe_path_or_file: link, non-directory ancestor, nonregular leaf,
  multiple hard links, oversized file, or observed identity/change race; null.
- assessment_error / cannot_read_asset: other assessment I/O failure; null.

Return 0 when all six match; 2 when assessment has missing/customized/unsafe rows;
1 when any row has assessment_error, taking precedence over 2. These statuses
describe reference assessment only: even status 0 never claims ownership,
rollback readiness or an applicable upgrade. Per-asset results go only to stdout;
no additional diagnostic is needed for their nonzero classification status.
Invalid request/root syntax returns 2 with sanitized stderr and no JSON. Missing
or unreadable root returns 1 with sanitized stderr and no JSON. Unsafe root
returns 2, including detected root identity replacement at the final recheck
(no JSON). Cancellation, unexpected assessment failure or output failure returns
1 with sanitized stderr and no success claim; do not claim atomic pipe output.

## Confined observation

ROOT must name an existing ordinary directory. Reject empty operands, NUL,
parent-traversal components and symlink components, including the root itself;
accept absolute or cwd-relative physical paths and ordinary dot/trailing-slash
forms. Do not clean away traversal before checking it or silently canonicalize
symlinks. On systems with /tmp or /var aliases callers select the physical path.
Pin directory handles; inspect each catalog path through pinned, no-follow
directory/file operations. Never open a device/FIFO/socket for a blocking read.
Reject hard-linked regular files even when their bytes match. No recursive scan.

Hash ordinary files with at most 32 KiB per read, a 1 MiB per-file limit, and
cooperative parent cancellation. Validate descriptor identity, type, link count,
size, mode and modification time before/after reading, plus the named path and
ancestor identities after observation; a detected change is unsafe. Inspect
special modes as well as permission bits. Recheck pinned root identity before
publishing the result. Use fixed diagnostics without raw file data.

This is a read-only observation under stable filesystem ancestry, not an atomic
snapshot or a defense against an adversary restoring all metadata between
observations. It cannot authorize later mutation: planning/apply must revalidate
ownership, trust and state under their own exclusion protocol. Arbitrarily slow
regular filesystem calls remain outside hard-deadline guarantees. Reading may
update filesystem access times; no application data, mode, index, cache, state,
temporary files, locks or backups may be written by this command.

## Independent qualification and reuse

Independent Ginkgo/Gomega tests invoke the actual compiled private request and
observe RED before production changes. Cover exact immutable blobs/modes, changed
content and mode, missing paths/ancestors, leaf/ancestor/root symlinks, hard links,
directories/FIFOs, oversized inputs, invalid operands, unreadable inputs where
platform permissions permit, forged version/ownership metadata and ignored extra
files. Prove deterministic JSON/counts/status precedence, no raw content leakage,
no subprocess/model activity and unchanged project contents/modes. Use controlled
filesystem collaborators for deterministic change/read/cancellation failures;
do not use sleeps as race readiness or require hostile timing to pass.

Keep production in one focused assessment package and one thin private adapter.
Reuse Cobra, Go filesystem primitives and already pinned x/sys when needed; add
no dependency or toolchain version. Separate read-only observation from statefile
storage, whose writable ownership/lock lifecycle must not be imported here.
Run focused race tests and complete source quality gates. Review security and
correctness independently before merging. Installed behavior remains unchanged.
