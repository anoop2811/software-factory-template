# ADR-0092: Plan controlled recovery restoration without granting authority

Status: Accepted for source implementation; active restoration remains gated.
Date: 2026-10-03
Decision: 91

## Purpose and prerequisite

After merged PR #118, provide a useful read-only restoration plan for one
strict recovery set. This refines the next controlled-restoration milestone;
it does not complete that milestone. The governing requirements are
specs/001-go-runtime-conversion.md:132, specs/001-go-runtime-conversion.md:168,
specs/001-go-runtime-conversion.md:319 and specs/001-go-runtime-conversion.md:422.

The v1 six-reference manifest proves saved reference bytes, not reverse
transaction ownership or compatibility with current runtime state. A missing
installed path may be a later user deletion. Current recovery locking does not
participate in runtime preflight; budget admission locks do not cover an entire
native process. Empty history and successful lock acquisition cannot establish
that an old process paused before admission is absent. Do not introduce active
restoration under a different flag to bypass these prerequisites.

## Source-built command

Reserve a separate bare --plan-restore marker and its attached spellings before
ordinary legacy dispatch. The strict accepted grammar is:

```sh
factory upgrade --plan-restore --migration-id ID [--json]
```

ID uses the existing 1..64-character ASCII recovery identifier grammar.
Require each option exactly once; bare switches reject attached values. Scalar
options accept detached or attached nonempty values. Reject creation, source,
ref, dry-run, inspect-backups, rollback, help, consent flags and other operands.
Legacy detached --source and --ref operands retain their ownership even when
a value equals --plan-restore. Independent planning/creation markers reserve
native handling and mixed modes refuse; private bridge precedence stays intact.
The physical current directory is the only installation root. Do not select
an arbitrary source, change cwd or run a colocated legacy upgrade script.

## Read-only assessment and reuse

Inspect only the requested set through the bounded, no-follow recovery reader.
Retain its strict format, complete-tree checks, ownership, private modes,
link refusal, byte limits and complete metadata/identity revalidation. Reuse
that reader and the catalog observer rather than duplicating manifest parsing,
reference hashes or per-harness policy. Keep source/recovery and observed
installed ancestry/leaf identities pinned or equivalently revalidated until the
whole plan completes; changed earlier observations invalidate the plan.

Only an integrity-checked set supplies its sorted manifest selection. Held
intact sets remain readable and are explicitly blocked. An absent, incomplete,
unrecognized, unsafe or oversized set produces a bounded blocked report with
no asset candidates. Unknown entries never become selected installation paths.
A recovery read/assessment failure uses status 1. Missing storage is reported
as missing without creating it. Do not inspect unrelated sets or runtime history.

For each selected installed path, report exactly one candidate action:

- retain_reference: ordinary content, type and complete mode match the catalog.
- restore_missing_candidate: path or safe ancestor is absent; absence grants no ownership.
- preserve_customized: an ordinary file has later content or mode differences.
- conflict: unsafe type, links, special modes, ownership or writable ancestry.
- assessment_error: the bounded installed observation could not complete.

Hash only bounded regular files; never open a FIFO/device or follow a link.
Customized or unselected content stays untouched. A special-bit mode is unsafe,
not a matching file after permission display normalization. These are proposed
reference actions, not proof of prior installation origin or permission to write.

## Deterministic report and refusal

Text and JSON have the same information. JSON has exactly these 17 fields:
schema_version=1, mode=restore_plan, coverage=partial, scope=g2-budget-loop-six,
migration_id, source_revision, target_revision, target_authentication,
recovery, assets, counts, blockers, restorable=false, rollback_ready=false,
activation_ready=false, applicable=false and prune_authorized=false.

recovery reuses the existing RecoverySet schema. Valid sets expose the compiled
reference revision and the manifest's descriptive target, labeled
operator_metadata. Invalid/absent sets use empty revision strings and
target_authentication=unavailable. Each sorted asset has path, action, reason,
reference and installed (classification plus observed metadata or null).
counts includes all five action keys, including zeros. Blockers always include
migration_ownership_unproven, transition_quiescence_unproven,
runtime_compatibility_unproven and activation_checks_pending. Add
recovery_not_integrity_checked, recovery_held, installed_conflicts or
assessment_failed when appropriate, in deterministic order.

Every complete plan returns status 2 because application remains ineligible;
assessment failure returns 1. Invalid arguments/IDs and unsafe or changed roots
refuse with status 2 and one safe diagnostic. Cancellation, I/O, descriptor-close
and output errors return 1 with one safe diagnostic. Never print absolute roots,
file contents, raw errors, unsanitized operands or imported arbitrary metadata.
No success result implies full installed rollback readiness.

## Mutation and compatibility boundaries

No create, write, chmod, lock acquisition, Git query/configuration, version
update, history read/reset, check execution, probe, model, download or process
launch is part of planning. Preserve backup bytes/modes, the Git index and ignore
files, every installed/user file and runtime history. Planning works without
Git or a valid unrelated execution configuration. Existing creation, previews,
private bridge calls, legacy dispatch and historical script paths keep their
contracts; update source-built help to describe the additive local operation.

## Outside-in qualification and progress

An independent spec-writer must observe compiled CLI RED before implementation.
Ginkgo/Gomega acceptance covers mixed installed actions, subset/full catalog,
text/JSON consistency, held/absent/invalid/incomplete/unsafe recovery sets,
customized modes/content, links, special files, privacy, missing Git, invalid
unrelated configuration, strict grammar, legacy operands, mixed modes, private
precedence and unchanged filesystem/Git/history snapshots. Independent internal
fault specs cover cancellation, bounded read failure, earlier source/destination
identity changes, ancestry changes and descriptor closure. Requalify existing
recovery inspection/creation when factoring shared observation lifetimes.

Use the existing pinned Go pack and libraries; no new version or dependency.
Run focused outer/compiled race, source quality checks and Linux/macOS CI, then
independent review with finding verification before merging. The 30-package
progress denominator stays fixed. PR #118 earns 18/30=60.0% after its merge.
This read-only prerequisite earns no extra controlled-restoration credit;
active restoration, reverse transaction evidence and transition exclusion still
have to be implemented and qualified before the final backup/rollback milestone.

## Qualification refinement: attribute changed observations accurately

Independent review on 2026-10-03 found that a changed installed path could clear
candidates by replacing the saved-set classification with unsafe. That falsely
attributed an installed-only change to the intact backup and suggested repairing
recovery storage. Discard the entire transient plan instead and return one fixed
changed-observations diagnostic. Use status 2 for dynamic identity/metadata/absence
changes, or status 1 when an already observed assessment failure takes precedence.
Static absent, incomplete or unsafe recovery sets still have their bounded blocked
reports. Preserve checked cleanup and status-1 precedence for close errors.
Observe a regression fail on the incorrect attribution before correcting source.

PR #119 review refinement: apply assessment-failure precedence inside retained
installed observation as well as after it. If a failed installed read and an
ancestor replacement occur within the same observation, its ancestry recheck
must retain the already observed assessment error. Final retained-pin validation
then discards the plan with status 1. Successful observations with changed
ancestry still refuse with status 2. Preserve the historical unpinned observer's
unsafe classification for changed ancestry; this refinement is planning-only.
Require a real installed-read/inode-replacement regression and an unpinned
compatibility control before changing the shared observer.
