# ADR-0080: Conservative Go migration action planning

Status: accepted for private source qualification; trusted ownership pending.
Decision: 79. Date: 2026-09-26 UTC.

## Purpose and honest scope

Extend ADR-0079's six-file reference assessment into a read-only comparison with
an explicitly supplied local target source tree. This advances the action-list
part of specs/001-go-runtime-conversion.md:156 while preserving uncertain files
as required by specs/001-go-runtime-conversion.md:176. A local source directory
and matching prior bytes do not establish authenticated release provenance or
installation ownership. Legacy .factory-version is editable and supplies neither.

The planner must therefore produce a useful but blocked candidate plan. It does
not accept a caller-provided "trusted" boolean, ownership manifest or a new
adoption/consent flag that launders local data into authority. Positive trusted
prior-origin integration remains pending, as do public preview and application.

Keep ADR-0079's existing three milestones and the 30-package denominator. The
second milestone comprises trusted prior-origin integration and target-action
planning. This slice qualifies only its target-action half: one sixth of the G4
ownership/preview package. Completion of this slice changes the aggregate from
46.6667% to 47.2222%, not to a completed ownership/preview package. No G2 rollout,
retirement, backup, rollback or other feature credit follows.

## Private request and observation

Under FACTORY_BRIDGE_PROTOCOL=1 accept exactly
`factory migration plan ROOT SOURCE`, two literal directory operands, including
dash-prefixed names. Keep existing assess behavior and public upgrade dispatch
unchanged. Use the existing Cobra private admission and scoped signal handling.
Neither tree may be mutated. No subprocess, model, network, temporary file, lock,
cache, configuration evaluation or recursive discovery is permitted.

Reuse the six compiled paths and immutable reference revision from ADR-0079.
Observe both trees with the same no-follow, single-link, bounded descriptor
reader; do not duplicate containment or hashing. Hold both root chains until
the final identity recheck. Both roots have ADR-0079's physical-path syntax,
mode/type/race/cancellation limits and stable-ancestry qualification. They may
name the same directory; this still gives no authorization. Inspect only these
paths, never a target-supplied manifest or an implicit list of target omissions.

Preserve full internal special-mode evidence so source setuid/setgid/sticky bits
cannot disappear when ordinary permissions are rendered. A safe source file is
usable for a candidate action only if its complete permission/special bits equal
the reference mode. Source bytes may differ: they describe a proposed local
replacement, not a qualified Go implementation or a trusted release. Record
source classification as relative to the old reference, not as proof of origin.

## Explicit intent and ordered action rules

The three retained compatibility paths are scripts/factory-budget.sh,
scripts/factory-loop.sh and scripts/lib/budget-config.sh. They must remain in a
complete target. The only retirement candidates are the three exact Python
paths scripts/lib/budget.py, scripts/lib/budget_adapters.py and scripts/lib/loop.py,
as specified by docs/migration/COMPATIBILITY_INVENTORY.md:50. Omission of any
other path never infers retirement. Python files present in SOURCE are retained
or replaced normally; no assertion of Go conversion follows from their absence.
Keep that retirement intent in a compiled reviewed transition table, independent
of source contents. A compatibility-path role names its retained contract, not
proof that arbitrary source bytes implement a working compatibility adapter.

Evaluate each row in this precedence order:

1. Either observation is assessment_error: action assessment_error, reason
   cannot_assess_path. Other rows still appear.
2. Either observation is unsafe, or an observed source file has invalid complete
   mode: action conflict, reason unsafe_path_or_source_mode.
3. Installed classification is customized: action preserve_customized, reason
   installed_customization. Even bytes identical to SOURCE remain a customization
   relative to the prior reference; equality to a target does not adopt ownership.
4. SOURCE is missing for a compatibility path: action conflict, reason
   required_compatibility_path_missing.
5. SOURCE is missing for a Python path: matching installed file gives
   retire_candidate / explicit_legacy_retirement; missing installed file gives
   absent / absent_in_both.
6. Installed file is missing and SOURCE is usable: add_candidate /
   missing_installed_path.
7. Both usable observations have identical bytes and complete mode: retain /
   unchanged_target. Otherwise replace_candidate / target_differs.

All destructive-sounding actions are proposals only. Candidate addition does
not authorize creating parents or overwriting a newly occupied path. Retirement
requires trusted ownership, qualified replacement, compatible adapters, quiescence
and verified recovery before any later apply can approve it. Neither command
touches .factory-version, user policy, runtime history or backups.

## JSON contract and status

Emit one newline-terminated JSON object with schema_version=1,
reference_revision, scope="g2-budget-loop-six", prior_origin="unproven",
source_authentication="unverified_local", ownership_authorized=false,
activation_ready=false, rollback_ready=false, applicable=false, blockers, assets
and counts.

blockers is exactly this ordered list of unmet prerequisites:
prior_origin_unproven, target_authentication_unproven,
runtime_qualification_pending, transition_quiescence_unproven,
verified_recovery_pending. Runtime qualification includes the candidate adapters
and their compatibility, not just a binary. These are unresolved on every plan in
this slice, including an unchanged tree. Neither valid-looking local metadata nor
environment variables can clear them. No file contents or absolute root paths
appear in the output.

assets has six rows in reference-path order. Each has path, role
(compatibility_adapter or legacy_implementation), action, reason, reference
{sha256,mode,bytes}, installed {classification,observed}, source
{classification,observed}. Observation metadata is ADR-0079's shape or explicit
null when unavailable. Source classification keeps ADR-0079's five labels; mode
validation additionally informs action conflict. Counts includes all eight action
keys, even zero: retain, replace_candidate, add_candidate, retire_candidate,
preserve_customized, absent, conflict, assessment_error.

Return 2 with the JSON plan because mandatory prerequisites remain blocked;
return 1 with the plan if any assessment_error occurs. No successful status 0 is
defined for this private planning slice. Invalid request/root syntax and unsafe
root/replaced root return 2 with sanitized stderr and no JSON. Missing/unreadable
roots, cancellation, unexpected failures and output errors return 1 without a
success claim; do not claim atomic pipe output. Per-row nonzero plan statuses
need no extra stderr. ROOT/SOURCE failures must not disclose raw file content.

## Independent outside-in qualification

Write independent compiled Ginkgo/Gomega core tests and observe RED before code.
Compare six unchanged references; generate changed targets, missing target Python
files and missing compatibility paths; preserve customized installed files even
when target-equal. Cover additions, mixed rows, exact counts/reasons/order and
all mandatory false authority fields and blockers. Poison local version/ownership
and target manifests must neither clear blockers nor select extra paths.

Qualify both roots for literal/invalid operands, symlinks, hardlinks, special
modes, nonregular and oversized files, missing/unreadable paths, unchanged bytes/
modes/trees and no subprocess activity. Reuse prior assessment regressions and
independent controlled faults to prove source-side read failures, cancellation
and replacement of either root while the other tree is being read. Keep live
native clients and paid model calls out of tests. Run focused compiled race tests,
full Go source gates, current-head platform CI and independent reviews.

Keep one shared confined observer, a small deterministic planner and a thin
private command adapter. Reuse existing dependencies; add no toolchain pin,
generic graph engine, public protocol or speculative trust infrastructure.
