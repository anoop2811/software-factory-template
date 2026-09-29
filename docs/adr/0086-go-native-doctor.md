# ADR-0086: Native Go factory health report

Status: accepted for implementation; qualification pending
Date: 2026-09-28
Decision: 85

## Context

Native init is merged in PR #112. The next user-requested conversion is doctor.
The source dispatcher still executes scripts/factory-doctor.sh. The legacy report
classifies gates and verifies hooks, adapter drift and break/fix proofs. Its
adapter comparison rewrites live files and restores snapshots, which is an
unnecessary risk for a diagnostic command. This closes only the doctor portion
of the existing G3 orchestration package in specs/001-go-runtime-conversion.md:533.

## Decision

Implement doctor behind the existing Cobra command without executing, interpreting
or requiring factory-doctor.sh. Preserve the report sections, classifications,
meaningful diagnostics and exit semantics: a missing configuration, missing core
hook or failed proof is a problem (status 1); inert choices and warnings alone
remain status 0. Legacy doctor ignores arguments; retain that invocation behavior.
Preserve Git-root discovery with cwd fallback, explicit FACTORY_CONFIG selection
(including paths relative to the selected root), flat first-key configuration
semantics and environment precedence for the review lane. Configuration is inert
data. Reuse the existing Go readers/export planning, process supervisor and
bounded discovery where their contracts match; do not fork parsers or introduce
new libraries, versions or model defaults.

Compute gate classification, hook executability, pack wiring, wiki presence,
CODEOWNERS reference checks and final counts in Go. Check effective Git hooksPath
with literal bounded Git commands, preserving the armed/inert/hijacked/absent
states. Configured check_command is reported as text, never evaluated by doctor.
Keep the legacy CODEOWNERS check's limited claim: it reports references, not a
proof of GitHub rule matching or remote branch protection. Use literal protected
path reference matching rather than interpreting configuration as a grep regex.

Retain shared sync-claude/sync-codex scripts, review-lane secret-name/pending
and scripts/selftest/run.sh as explicit subprocess boundaries until their own
conversions. Never invoke a paid model or install dependencies. Supervise children
with empty input, bounded output and cancellation; reuse owned-group cleanup.
Git discovery/probes have a five-second allowance; sync/review helpers have a
one-minute allowance; break/fix proof has fifteen minutes. Keep existing 16 MiB
per-stream capture bounds. Timeout, cancellation, output overflow or uncertain
child ownership cannot be reported as healthy. Handled SIGINT/SIGTERM return 1
with a diagnostic and stop owned children. Nonzero sync exits must be visible as
warnings, never as adapter agreement. Nonzero or unavailable review status must
be reported as unverified, never as a secret known to be present.

Adapter drift must not replace live adopter files. Prepare a private temporary
snapshot of only the canonical inputs, required shared script/library assets and
existing generated adapter paths. Run retained generators in this scratch root,
with config pinned to the equivalent scratch snapshot and Git routing cleared/
parent discovery bounded. Compare generated outputs to the captured live state,
including mode/type/link-target changes and newly created outputs. A legitimate
CLAUDE.md -> AGENTS.md relative link is compared as a link, not followed. Do not
copy .git, node_modules, recovery archives or the whole project. Preserve user
adapters, existing backups and source metadata; remove only owned scratch data.
Refuse/skip unsafe adapter inspection with a visible warning rather than follow
links outside the selected inputs, read special files or mutate a live tree.
The scripts are trusted repository tools, not sandboxed arbitrary executable code.
Their own break/fix proof remains a retained validation boundary; this port does
not promise to sandbox custom user selftests.

Bound diagnostic input reads to 16 MiB per file, 4096 entries per inspected tree
and 64 MiB total adapter snapshot data; check cancellation during traversal.
Ignore recovery copies by inspecting only the fixed active asset manifest. Do not
silently claim a clean report when a read/limit failure prevents inspection.
Preserve the immediate non-TTY progress message before a proof starts. For TTY
output, provide visible progress while the proof runs, then print its tally and
elapsed duration; incidental temporary names and exact elapsed timing are not
byte-stable contracts. Failed proofs include their captured diagnostics. Output
write failures are nonzero. Missing optional sync/adapters retain explicit skip
reporting. A missing optional shell timing/hooksPath library must not disable a
native capability that no longer depends on that library.

No installation/cutover, config mutation, Git config change, backup creation,
legacy retirement or new migration authority is introduced. Existing installed
entrypoints remain unchanged. Update source-runtime help and migration docs to
identify doctor as native while retaining the explicit subprocess boundaries.

## Qualification

Independent Ginkgo/Gomega compiled-command acceptance must fail against the old
script route first. Cover healthy/inert/stale/missing-hook reports, no-config,
config override and inert literal values, pack wiring, CODEOWNERS and Git hook
states, review status/failure, failed proof, ignored argv, no legacy script,
read-only drift including user edits/new generated files/symlinks/backups,
bounded unsafe inputs, cancellation and TTY/non-TTY progress. Compare representative
reports with a frozen legacy script using local fixtures and controlled helpers;
normalize only incidental paths/timing. Include a real retained-sync/proof path
without network or paid model calls. Keep evaluators separate from production.
Run relevant race/quality checks and existing shell selftests; independent review
and green exact-head CI precede merge and the next command conversion.

Compatibility clarification from qualification: split pack/path lists on the
shell default space, tab and LF boundaries. Do not expand filesystem globs from
configuration values or broaden separators to Unicode whitespace.
