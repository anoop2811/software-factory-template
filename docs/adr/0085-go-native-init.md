# ADR-0085: Native Go initialization orchestration

Status: accepted; source command qualified and merged in PR #112
Date: 2026-09-28
Decision: 84

## Context

The user requested native Go init, doctor, report, metrics, review-lane and
configuration migration. Complete each command and merge its reviewed PR before
starting the next. The current source-built dispatcher still delegates init to
scripts/factory-init.sh. Source conversion and installed runtime activation
remain different milestones under specs/001-go-runtime-conversion.md:91.

## Decision

Implement init in Go behind the existing Cobra command. Do not execute or parse
factory-init.sh as implementation. Preserve the prompt order and noninteractive
input, explicit confirmation, pack selection/deduplication, Java build selection,
model/profile defaults, generated configuration, template copy manifest,
placeholder replacement, Makefile/ignore block preservation, README retention,
optional dependency installation and mandatory installed gate selftest.

Template assets are selected relative to the invoked executable using the same
source-layout contract as other commands. No download, paid model invocation,
Git commit, runtime activation, legacy retirement or upgrade transaction is
introduced. The installed factory remains the existing template entry point
until the separate activation contract qualifies. Build the source command as
factory-go beside the tracked factory shell asset; init must not install its own
compiled executable as a substitute. Refuse a missing/non-shell factory asset
before writing rather than silently activating an unqualified runtime. Missing required assets fail
clearly; optional assets retain their documented optionality.

Keep the legacy optional-target argument behavior (last unrecognized operand is
the target), repeated/comma-separated packs and Java build-tool validation. The
confirmation accepts a response beginning y or Y, matching the current prompt.
Invalid pack names are refused rather than interpreted as filesystem paths.

Prompts read the controlling terminal when stdout is a terminal and one can be
opened; otherwise use stdin, including EOF defaults. Preserve meaningful messages,
configuration/artifact bytes and exit semantics; incidental shell-tool diagnostics,
absolute temporary names and timestamps are not stable output contracts. Literal
values must remain data; Go replacement must not reproduce sed expression injection.
Require valid UTF-8 in prompt answers and consumed initialization environment values
before normalization or JSON encoding can silently replace invalid bytes.
Bound each answer to 64 KiB and refuse an oversized unterminated answer without
waiting for EOF. Bound individual template inputs to 16 MiB and each discovered
asset directory to 4096 entries, and the target root sidecar scan to 8192 entries,
checking cancellation while processing them. Each external stage has a ten-minute
deadline and separate 16 MiB stdout/stderr limits; reuse the supervisor's bounded
five-second owned-group cleanup. Child output is captured for that supervision,
so native init owns the optional interactive review-model question: after the
main Proceed confirmation and before publication, when review is enabled and
no review model is already chosen, show the provider/default and accept a model
(or Enter for the existing default). Noninteractive input retains blank/default
behavior. This intentionally moves that choice before writes; do not lose the
choice merely because the retained review helper now sees captured output.
Retain the helper's pending/action-required output so unresolved setup is visible.
Existing defaults are compatibility inputs, not new claims about latest models or
tool versions. Do not introduce dependencies or change pins for this port.

Keep the existing explicit external boundaries: npm dependency installation,
shared harness synchronization scripts, review-lane enable/pending until its own
conversion, and installed validation/selftest scripts. Execute fixed commands with
literal arguments and target cwd. Pin child FACTORY_CONFIG to the target file
and prevent inherited Git routing or parent-repository discovery from redirecting
initialization to another project, including a target Git config redirecting core.worktree; this is an intentional confinement correction,
not permission to execute an outside project's configuration. Preserve target
worktree operation and do not change the caller's environment. A bounded read-only
Git preflight must refuse before publication when the target's effective Git root
resolves to a different physical directory. Ordinary non-Git targets and valid
worktrees remain supported. Preserve Bash export-state semantics: when a collected
or computed initialization variable was originally exported, child processes see
its updated value; variables that were not exported do not become new environment
overrides merely because Go stores them in the answer map. Required selftest failure returns nonzero, while
existing advisory dependency/sync failures stay warnings. Signal cancellation must
stop owned child work and must not hang on blocked inherited input. Output-limit,
timeout, cancellation and unconfirmed child ownership are fatal even at otherwise
advisory stages. Handled signal cancellation returns status 1 with a cancellation
diagnostic; this is an explicit native failure contract rather than an unhandled
shell signal exit. Preserve ownership errors when reporting simultaneous failures.

Initialization is not authorization to retire legacy assets. Preserve existing
user-facing backup behavior for supported regular files while making filesystem
boundaries explicit: refuse unsafe links/special files on planned source/write
paths before writing target files; do not follow them outside the selected roots.
Validate all planned destinations before publication, including known shared-sync
outputs (.claude, .mcp.json, Codex/agent files and CLAUDE.md), temporary sidecar
paths and the dependency-installation directory. Allow the deliberate existing
CLAUDE.md to AGENTS.md relative link as an explicit retained adapter contract;
refuse unrelated links. Validate generated role names as single filename components.
These controls are not a sandbox for arbitrary npm dependency lifecycle code.
Observe each exclusively created publication temporary's identity. Revalidate its
prepared identity before rename; on failure remove only the matching owned
unpublished temporary. Never delete an unknown replacement or unlink the old
temporary name after a successful rename. Check file close before publication.
These are observed-drift checks, not a hostile-parent atomic compare-and-swap.
Refuse source/target overlap.
Do not create even the target directory before explicit confirmation. Pack names
must be the supported go/typescript/java names (none selects nothing), never paths.
Existing ordinary-file replacement backups must be exclusive and collision-safe,
include the old factory.yaml, use private inert 0600 copies, and never overwrite a previous backup. Byte-and-mode
identical destinations are left untouched without another backup, so a repeated
identical initialization does not accumulate redundant copies. These are
initializer content-preservation backups, not the migration recovery inventory;
retirement/retention remains a separate migration obligation. Refuse multiline or
quote-containing configuration answers that the existing flat grammar cannot
represent unambiguously, before any installation write. Reject CR/LF in selected
root paths while retained shell boundaries cannot preserve them. Unquoted flat
fields must round-trip through the shared reader without comment/whitespace loss.
Protected paths used in generated shell commands must be relative and consist of
ASCII letters/digits, slash, dot, underscore or hyphen, without parent traversal;
refuse other spellings before writes rather than embedding executable syntax.
Version answers embedded in quoted workflow scalars admit only ASCII letters,
digits, dots, underscores, hyphens and plus signs. JSON string substitutions must
escape data for JSON; literal backslashes must not become JSON escapes or code.
CODEOWNERS owner input may be a space-separated owner list but must not contain
comment markers or escape/control syntax that changes the generated rule. This
is syntactic admission, not remote account validation; retain the existing empty
answer behavior. The consumed review secret name must be an ASCII identifier
starting with a letter or underscore and continuing with letters, digits or
underscores; it must never introduce syntax into the retained workflow renderer.
Preserve user README, Makefile targets and ignore rules. Never claim transaction
rollback or durable migration recovery for this initializer. No broad cleanup.

## Qualification

Independent Ginkgo/Gomega compiled-command tests must fail against the old script
route before implementation. Exercise no-script dispatch, abort/no writes, EOF,
plain and polyglot installs, Maven/Gradle choice, version/model inputs, literal
substitution, rerun preservation/backups, invalid inputs, missing assets, unsafe
paths, child failure and cancellation. Compare representative generated artifacts
with the legacy implementation using isolated local fixtures and controlled external
commands. Do not access npm registries or invoke paid models in acceptance tests.
Existing CLI tests must distinguish native init from still-script-backed commands.
Run focused race acceptance, Go quality checks and existing factory selftests;
independent correctness/security review precedes merge.

## Consequences

This closes native init orchestration only after evidence and merge. Adapter sync,
review-lane logic, installer distribution and legacy-script retirement stay visibly
separate work; no command is called fully shell-independent while these boundaries
remain. Existing installations continue to use the legacy initializer until the
qualified distribution/cutover stage.
