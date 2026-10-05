# ADR 0096: freshly qualified interrupted publication recovery

- Status: accepted source implementation refinement; no installed recovery consumer
- Date: 2026-10-04 UTC
- Decision: 95

## Purpose and source boundary

After merged PR #122, implement R2.2: freshly qualified recovery of one interrupted
durable publication. This refines specs/001-go-runtime-conversion.md:161,
specs/001-go-runtime-conversion.md:318 and specs/001-go-runtime-conversion.md:321.
Public controlled rollback, complete installation compatibility, activation,
script retirement, retention and release qualification remain separate work.

Abrupt termination loses the old live capability. A journal is an observation,
not an authenticated receipt or reconstruction of that capability. V1 records
contain no pending-marker identity. Recovery grants new authority over currently
checked evidence through explicit fresh consent and independently known images.
Operation IDs select evidence; they are not credentials. A parser cannot
authenticate a self-consistently forged local record written before the proposal,
and this component makes no unchanged historical ownership claim under AC 3.3.

## Additive source API

Use the existing assessment package and its source-only external compiled clients:

```go
type InterruptedRecoveryRequest struct {
    MigrationID string
    OperationID string
    Path string
    Direction string
    Replacement []byte
    AfterReference Observation
    UnbridgedQuiescent bool
}

func ProposeInterruptedRecovery(context.Context, string, InterruptedRecoveryRequest) (InterruptedRecoveryProposal, error)
func BeginInterruptedRecovery(context.Context, string, InterruptedRecoveryRequest, string, map[string]string) (*InterruptedRecovery, error)
func (*InterruptedRecovery) Complete(context.Context) error
func (*InterruptedRecovery) Close(context.Context) error
```

The second string to Begin is the proposal digest explicitly confirmed by the
caller. Direction is exactly forward or reverse. Identifiers retain the existing
recovery grammar; paths select exactly one existing compiled catalog leaf.
Replacement is bounded at the existing 1 MiB asset limit and is copied at grant.
AfterReference must independently come from caller-qualified source or an
authenticated artifact, match those replacement bytes and the catalog ordinary
mode, and agree with the checked record. Never derive both the replacement and
its trusted reference from journal fields. The saved original must match the
compiled before-reference. Refuse identical before/after observations in this
initial bounded API rather than guessing a historical phase from identical bytes.

UnbridgedQuiescent is a required explicit operator/trusted-caller affirmation,
bound in consent. It is not measured process absence, a detected fact, or
authority by itself. It never overrides activity, active accounting, uncertain
checkpoint state or unsafe storage. The caller must separately establish
unbridged-process quiescence, complete target trust/dependency coverage and
semantic compatibility of the intended runtime. No age, PID probe, empty ledger
or available flock proves those obligations.

InterruptedRecovery is opaque. Value aliases share one lifecycle, mutex,
selected direction, checked pins, candidates and closure. No serialized/imported
handle or environment override supplies authority. Existing Publication APIs,
read-only inspectors/planners and normal guards preserve their contracts.

## Read-only proposal and fresh consent

Propose opens only existing storage through confined no-follow readers. It never
creates or repairs infrastructure, acquires locks, syncs files, launches Git,
checks or models, downloads, or changes files. Missing/unsafe/ineligible evidence
returns one safe refusal; operational/cancellation/close errors return status 1.

The proposal has these observable fields: SchemaVersion, Mode, Scope, Purpose,
MigrationID, OperationID, Path, Direction, CurrentImage, ProposalDigest,
RootIdentity, Current, Record, RecordIdentity, PendingIdentity, Recovery,
StateCompatibility, UnbridgedQuiescence, Restorable, RollbackReady,
ActivationReady, Applicable and PruneAuthorized. Reuse RootIdentity, AdoptionAsset,
Observation, AssetIdentity and RecoverySet for those observations. SchemaVersion
is 1; Mode is interrupted_recovery_proposal; Scope is
g2-budget-loop-single-publication; Purpose is fresh_current_image_resolution.
CurrentImage is before or after. StateCompatibility is g2-state-v1_checked and
UnbridgedQuiescence is operator_affirmed. Every authority flag is false.
JSON uses the corresponding snake_case field names. Never echo absolute roots,
replacement bytes, history, environment or raw errors.

The deterministic, domain-separated digest binds every request field, including
the trusted after-reference, replacement observation, direction and affirmation.
It also binds complete actual root/control/controlled-ancestor identities and
metadata, selected current identity/bytes/mode, exact pending identity/empty
content, selected record identity/bytes, the complete checked publication
namespace, selected saved-set/manifest/hold/payload observations and actual
budget/checkpoint identities/bytes or their checked absence. Revalidate the whole
observation before checked descriptor closure; return a digest only when both
validation and closure succeed. No stale or partial proposal
can grant authority. Identity metadata means device, inode, type, complete mode,
owner/group, link count, size and modification/change timestamps; omit access
timestamps changed by these same reads. Above the selected physical root, retain
the existing stable ancestry identity/mode/ownership boundary rather than binding
unrelated system-directory listing timestamps.

## Existing exclusion and pending evidence

Add transition.ExclusiveRecovery(ctx, root) returning (*Guard, error) as the
separately named recovery-only acquisition in the existing transition package. Reuse the permanent flock and controlled-directory machinery;
do not create a second protocol or weaken Shared/Exclusive admission. Require
existing safe project, .factory, permanent runtime-transition.lock, private
runtime-activity directory and an existing empty single-link owner-only regular
runtime-publication.pending file. Missing infrastructure refuses; do not create,
repair, chmod, truncate, replace or unlink controls.

Acquire the same existing nonblocking exclusive flock, refuse every activity
entry or inspection failure, and validate pending while held before returning.
The guard grants only exclusion. Assessment separately retains the exact pending
pin and owns its qualified resolution; pending is not a permanently required
guard pin that would make legitimate removal invalidate guard closure. Guard
lifetime checks preserve root/control/activity identity and the same lock inode.
Recovery-only acquisition and later Check must also preserve its observed empty
activity directory across pending qualification and other fallible boundaries.
An actual late activity entry cannot be admitted merely because its directory
keeps the same inode and ordinary metadata remains safe. Retain and revalidate
meaningful unchanged directory metadata or freshly confined enumeration; never
reuse an exhausted directory-read offset as proof of emptiness.
Normal Go/Python shared and fresh exclusive work still refuses every pending entry.

Begin recomputes the full proposal under that guard and requires the exact fresh
digest. Retain all positive pins and absence observations for later checks.
Requalify the existing exact local backup exclusion before any journal mutation
through the bounded supervised Git query machinery from ADR 0095. Require ignored,
untracked backup/record paths and readback/sync unchanged local exclusion. Query
ownership uncertainty preserves typed process evidence and pending. Never edit
ignore files or the Git index, create a record slot or consume additional quota.

## Checked state and consistency

Read existing private budget.json and loops.json with pinned no-follow bounded
files, without store helpers that create, repair or lock state. Reuse the existing
budget.ParseHistory and loop.ParseHistory schemas. Their schema-one readability
and lack of unresolved G2 work is the compatibility claim; it does not prove that
arbitrary replacement code or a complete older installation can read that state.

Add budget.History.HasUnresolved(ctx) and loop.History.HasUnresolved(ctx), each
returning (bool, error) on validated History values. Preserve existing
HasActive/controller semantics. Budget active rows block. A
completed row with a process PID but no recorded exit also blocks. A completed
historical PID with recorded exit is permitted; no PID liveness inference occurs.
Loop active or uncertain rows, or retained process_pid, block; owner_pid remains
descriptive history. Missing histories are allowed as checked absences.
Malformed/unsupported, oversized, linked, unsafe or changing state refuses.
Never rewrite, rewind or prune accounting/checkpoints, including consumed attempts,
unknown fields and terminal evidence.

Use the complete bounded strict publication reader. Require exactly the requested
existing record, matching operation/path/before/known-after/saved-set identity.
Refuse held selected recovery sets, any other nonterminal record, unsafe/unknown
namespace contents, unavailable/incomplete saved input and ambiguous evidence.
Other checked terminal records remain inert and untouched.

Imported identities are consistency restrictions, not positive historical
ownership. Require phase-appropriate current device/inode/type/complete mode/
size/mtime and exact known bytes. Full recorded identity applies where it was
observed after publication or for the unchanged original. Prepared candidate
ctime may change in rename; it is descriptive, not historical continuity.
Instead bind complete actual current metadata in the fresh proposal, grant and
every later validation. Equal-byte foreign inodes, absent files, changed ancestry,
different bytes/modes and post-proposal changes remain preserved conflicts.
When freshly recovering a prepared after-image, describe its newly checked current
ForwardIdentity before producing a subsequent reverse-prepared or published record;
do not carry the old pre-rename ctime as if it were a post-publication observation.

## Supported resolution matrix

| Recorded phase and checked current image | Permitted fresh direction |
|---|---|
| prepared or forward_prepared, original before-image | Reverse abort only |
| forward_prepared or forward_published, after-image | Forward completion or explicit reverse |
| reverse_prepared, prior after-image or candidate before-image | Reverse only |
| forward_completed, after-image with pending | Forward pending resolution only |
| restored or aborted, before-image with pending | Reverse pending resolution only |
| Every other combination | Preserve and refuse |

No forward replacement is applied from a currently original file in this slice.
Aborting restores old-image admission; a later new transaction owns any new work.
No absent-path creation, batch activation or recovery from an incomplete/unknown
journal is authorized. Preserve orphan siblings; their descriptive names alone
never authorize cleanup or adoption.

## Completion, retries and closure

Complete operates only under this newly granted capability and selected direction.
Recheck held ancestry, all state/absence evidence, selected current, saved input,
record, pending and guard before fallible publication/completion boundaries.
Reuse shared exclusive staging, candidate reconciliation, durable record writes,
image readback and sync, checked pending removal and resource closure. Do not
duplicate parsers, errno policy or a second file-publication engine.

If reverse publication is required, durably record its actual prepared candidate
before active rename. Select the direction before its first journal mutation.
An already desired checked image needs only durability/terminal record resolution,
with no redundant active rename. Terminal record and selected image/parent must
be read back and durable before unlinking only the exact newly checked pending
entry and syncing its parent. No chosen direction may silently change on retry.

Same-handle retries retain only actual newly prepared/observed candidates and
complete their selected direction; foreign edits and equal-byte replacement
inodes refuse. An interrupted retry loses the new capability too: another fresh
proposal/consent/grant is required. Terminal record plus pending is incomplete;
terminal record with absent pending remains inert inspection, not a new recovery
capability. A post-unlink parent-sync failure never recreates the pending marker.

Zero/nil, closed and successfully completed handles refuse further mutation.
Close releases every owned resource/lock even under cancellation and preserves
primary typed active-publication/process uncertainty with cleanup errors. It
never completes, restores or removes pending implicitly. Ordinary observation,
read/write/sync/close/cancellation errors are operational 1; missing/unsafe or
changed evidence is refusal 2. Cleanup errors retain operational precedence.
Recovery-only guard errors retain a safe typed distinction between explicit
metadata/lock conflicts and observation/cancellation failures. Preserve underlying
error classification for assessment to reuse its existing errno policy; do not
change ordinary guard error contracts or duplicate a second errno policy.
Private fixed diagnostics expose no sensitive bytes or filesystem roots.

Failed Begin cleanup must distinguish borrowed pre-existing pending evidence from
an incomplete returned capability. Before grant, release every owned resource
without manufacturing an incomplete-publication error solely from that borrowed
marker. Preserve the primary refusal, real cleanup errors, cancellation and typed
query-ownership uncertainty. After a returned grant, ordinary incomplete Close
diagnostics and typed publication uncertainty still apply.

The confined active/record directory qualification must classify actual Stat
observation errors before considering identity mismatch, with checked closure
and cleanup precedence. Private per-operation collaborators may wrap the real
os.Root.Stat and retained-file Stat calls for deterministic actual-operation-then-
reported-error qualification; defaults remain native and no test-only export is
introduced. Interface-only wiring precedes independent property RED and fixes.

## Outside-in qualification and limits

Independent Ginkgo/Gomega authors own only new behavioral test files. Observe
missing-API compile RED separately, then real runtime RED using interface-only
unsupported stubs before implementation. External compiled clients exercise the
actual production API, filesystem, distinct process and consent flow.

Observe acknowledged real SIGKILL at prepared, forward/reverse prepared before
and after active rename, forward published and terminal-record-before-pending-
removal boundaries. Qualify fresh resolution, repeated interruption, no redundant
rename and remaining ordinary Go/Python admission barrier. This is process-death
evidence, not power-loss durability. Preserve immutable saved sets, histories,
holds, unrelated records/files, orphan siblings, Git index and ignore bytes.

Cover stale consent, changed current/record/pending/ancestry/state, known reference
mismatch, unsafe storage, held/ambiguous evidence, activity, active/uncertain state,
historical budget PIDs, retained loop PIDs, copied aliases, direction restrictions,
cancellation and real-I/O collaborator faults with actual native operations before
an injected failure. Assert checked descriptor/flock release and status precedence.
Requalify publication/recovery/transition and relevant budget/loop parser behavior.
Run race, pinned Go-pack quality, structural gates and Linux/macOS hosted checks.
Apply workflows/review-diamond.md and verify findings before they count.

No installed command/dispatcher or default changes, public rollback, model call,
new dependency/tool pin, automatic cleanup, activation or release readiness.
Full AC 3.2 and 3.3 remain pending the installation consumer and qualification.

## Measurement

Keep the fixed 30-package denominator. Qualified and merged PR #122 earned
(18 + 1/9 + 1/18) / 30 = 60.5556%, reported 60.6%. Qualified AND merged R2.2
earns only its existing equal 1/18 package:
(18 + 1/9 + 1/18 + 1/18) / 30 = 60.7407%, reported 60.7%.
Public R3 and all activation/retirement/retention/platform/pilot/release gates
remain open. Do not award review fixes, planning or unmerged source extra credit.

## Cleanup and open-boundary refinement

A recovery guard validation conflict remains a refusal when its actual resource
closure succeeds. Failed-grant cleanup must not relabel it as failed closure.
Inspect compound errors across the complete error tree: any actual close failure
or cancellation retains operational precedence over an accompanying conflict.
Private native-default Guard.Close collaboration may qualify actual guard closure
followed by one reported error; it grants no authority and introduces no test-only
export. Reuse the existing errno policy for each observation, not a new classifier.

If a previously qualified directory/file disappears or becomes a symlink before
confined Openat, classify its real ENOENT/ELOOP as missing/unsafe refusal. Preserve
the changed evidence and close all descriptors/flocks. Other actual open errors
remain operational. Requalify existing shared writer consumers after correction.

## Constructor and cleanup error composition

Failed Begin applies operational precedence across both the primary constructor
error and its cleanup error, not only inside Guard.Close. If either non-nil
branch contains an operational failure or cancellation, expose a status-1
Failure before joining both branches. Preserve the original error trees for
errors.Is and errors.As, including cancellation and query-ownership uncertainty.
Conflict-only pairs remain status 2; successful cleanup adds no synthetic failure.
Keep this composition private to failed interrupted-recovery grants and retain
the shared ErrorStatus contract. Cleanup still closes all owned resources without
altering borrowed pending, current images, records, saved input or activity.

Independent Ginkgo/Gomega regression cases must reach the constructor under the
real exclusive flock, then combine actual late activity with primary cancellation
or an actual native observation followed by a reported I/O error. Include the
reverse precedence case and conflict-only/eligible positive controls. Assert
primary error reachability, checked descriptor/flock release and preservation
before status; demonstrate runtime RED before the production correction.
