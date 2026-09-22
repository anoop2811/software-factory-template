# ADR-0069: Interoperable Go budget ledger and admission

Status: accepted for implementation under specs/001-go-runtime-conversion.md.

## Scope and immutable baseline

Decision 68 continues G2 with storage, planning and atomic admission. The source
baseline is commit `4cc771e894e11d5024032106aeb0ef788997bd29`,
`scripts/lib/budget.py`: configuration, validate_history, Ledger, make_plan,
report and the reservation/publication portions of run. The approved feature
specification remains canonical, particularly FR-005 through FR-009 and FR-030.

Implement internal/budget. Do not add a public or private CLI mutation/launch
command, invoke a native CLI or change the installed Python controller. Test-only
drivers exercise the component through compiled boundaries. This slice advances
the existing budget-controller work package; it is not another completed package.
Native preflight/execution, controller output, loop fingerprints and installation
remain separate integration obligations. No legacy file is retired here.

## Compatibility and qualification

Preserve schema 1 and original key presence, nulls, integer types and unknown
top-level/run/token fields. Schema, attempt, owner PID and token counters must
not accept booleans or integral JSON floats where Python requires int. Preserve
exact large integer counters rather than decoding them through float64. Preserve
the baseline's permissive active-running row semantics; do not silently repair
historical records. Report filtering and totals match report, including elapsed
for all selected rows, reserved time for active rows, and cost completeness based
only on missing/null estimates. Planner blockers retain baseline order and text.

Admit the bounded scalar UTF-8, finite JSON domain: at most 32 MiB and 512 nested
containers. Reject unsupported nonfinite values, unpaired surrogates, oversized
or excessive-depth inputs before mutation. Unknown JSON remains data; never
evaluate or round it. Reject arithmetic overflow rather than admitting from a
nonfinite aggregate. Integer limits preserve finite-convertible Python integers.
Configuration retains defaults, enums, decimal integer rules and finite positive
float rules, including ordinary Python decimal exponent/underscore forms.

Float aggregation is qualified against CPython 3.12's compensated sum, the local
immutable-source oracle runtime; integer-only sums remain exact. Older Python
float rounding differences remain an installed-controller compatibility question,
not permission to claim all historical interpreters qualified.

## Read-only planning and reports

Reading absent history returns schema 1 with an empty runs array without creating
.factory, budget.lock or budget.json, or changing modes. Report does not validate
unrelated execution configuration. Validate identities and configuration before
any mutation. Plan does not reserve spending or authorize a native launch.

Attempts and charged session time are scoped to session/task. Concurrency, stale
owner detection and unconfirmed process blockers cover the whole checkout.
Active runs charge reserved_seconds; completed rows charge elapsed_seconds for
admission. All attempts remain consumed. Cost stop/warn affects only estimated
USD concerns; hard attempt/time/concurrency limits always block. Strict USD
ceilings remain unsupported. Model/services/cost labels remain baseline values.

## Shared storage and lock ownership

Use .factory/budget.json and the same persistent .factory/budget.lock inode as
Python fcntl.flock. Never unlink, rename or replace the lock as an unlock/recovery
operation. Use exclusive flock with nonblocking attempts and context-aware waits;
cancellation/deadline prevents admission after lock acquisition as well as before.
Do not use a Go-only mutex or a different fcntl locking protocol as a substitute.

The qualified storage environment is an owner-controlled project directory on a
local Linux/macOS filesystem supporting flock, atomic rename and file/directory
fsync. Concurrent cooperating Go/Python operations are supported. Hostile same-UID
namespace replacement, noncooperating manual edits and network filesystems are
not claimed safe; the existing Python participant cannot enforce that boundary.

Pin the storage directory and lock descriptors; refuse symlinks, nonregular or
hard-linked files, including FIFOs without blocking on open. Validate owner and
identity before chmod/mutation and after waiting for the lock. Preserve lexical
root traversal semantics, including symlink/parent components. Directory mode is
0700; lock, temporary and published history files are 0600. Read-only calls do
not repair modes. Check actual bytes read against the bound, not only stat size.

Inside the lock, read the current history, re-evaluate admission, append exactly
one reservation and durably publish it before returning permission to proceed.
Two contenders for the last slot must yield exactly one admitted reservation.
Blocked admission may create lock infrastructure, matching the locked legacy
phase, but must not append a run or rewrite history. Future controllers still
perform initial read-only admission before native preflight and this locked step.

Write through an exclusive private temporary file, sync it, close it, atomically
rename it over history and sync the directory. Validate and bound encoded output
before publication. Pre-rename failure preserves original history and removes
only the operation's owned temporary file. A post-rename failure is ambiguous
publication, not rollback: return a typed error retaining run/PID identity and
possible-commit status. Never automatically retry admission or launch on error.

## Reservation lifecycle

Admission assigns a unique run ID, current owner PID, UTC timestamp, next attempt
and min(per-run timeout, remaining session time) reservation. Metadata is initially
unknown. It does not launch a process. PublishPID happens only for that owner's
active reservation and must complete before a future executor delivers prompt
bytes. Refuse missing, completed or foreign reservations and conflicting PID
replacement; publishing the identical PID may be idempotent.

Finalize re-reads under the same lock and patches only owned completion fields
on the current row. Preserve identity, attempt, reserved time and unknown fields
added since admission. Do not upsert a disappeared row, replace a stale snapshot,
or silently finalize an already completed/foreign reservation. Persist only
allowlisted accounting metadata; no prompt, response, transcript or raw error.

Ownership uncertainty is separate from leader exit. If group/drain ownership is
unconfirmed, retain an active timeout/launch_error row, process PID, attempt and
time reservation; project exit_code to null and tokens/cost/completeness to
unknown/false so Python schema-1 readers accept it. A known leader exit code is
not permission to free uncertain ownership. Add a fixed recovery warning.
Timeout/interruption/output-limit or incomplete accounting clears token/cost
claims. Completed execution with failed/incomplete metadata becomes failed.
Only confirmed completion releases concurrency and sets ended_at. Cost concerns
are re-evaluated after publication data is prepared, as in the baseline.

## Deliberate tightenings and deferred activation

Unlike baseline write, encoded writes are bounded to what the next read accepts.
Unlike baseline blocking flock, waiting honors context cancellation/deadline.
Fresh-row finalization preserves concurrent unknown metadata rather than replacing
the baseline's stale snapshot. Explicit uncertain-ownership input prevents a
confirmed leader exit from hiding unresolved group/drain ownership. Typed ambiguous
publication prevents callers from treating an fsync error as a rolled-back write.
These are specified corrections, with independent regression coverage.

Public command activation, shell configuration/model routing, full controller
output, loop integration and runtime retirement require their own acceptance.
Default cutover still requires recoverable installation, gitignored predecessor
backups, retention cleanup, four-target qualification and the manual adopter pilot.

## Acceptance and provenance

Independent Ginkgo/Gomega compiled-driver tests precede implementation. Compare
schema/config/plan/report with the immutable Python source and explicit expected
values. Test Python-to-Go and Go-to-Python histories and actual flock contention
in both directions with readiness handshakes. Test two contenders, canceled lock
wait, unsafe paths, exact bounds, lifecycle ownership and persistence failures.
Private per-instance storage collaborators may inject failures; no mutable global
or production environment fault switch. Prove a negative control before claiming
a regression check. Test fixtures are not shipped commands or runtime fallbacks.

Official API references fetched 2026-09-21 UTC:
[unix.Flock](https://pkg.go.dev/golang.org/x/sys/unix#Flock) and
[os.File.Sync](https://pkg.go.dev/os#File.Sync). Existing module/tool pins remain;
this slice does not upgrade dependencies. Run the full source quality gate,
targeted race checks and independent correctness/security review before delivery.

## Internal API

- Configuration(environment map[string]string) (Config, error). Config holds
  Enabled bool, Action string, MaxAttempts/MaxSessionRuns/MaxConcurrent *big.Int,
  TimeoutSeconds/SessionSeconds float64 and EstimatedUSD *float64, with baseline
  JSON field names. Callers supply validated configuration; methods revalidate it.
- Request holds Session, Task, Harness, Role, Model strings and MaxCostUSD *string.
- ParseHistory(context.Context, io.Reader) (History, error) validates bounded
  input; History and Record expose JSON marshaling without mutable internal maps.
- MakePlan(context.Context, Request, Config, History) (Plan, error) and
  Report(context.Context, History, session string) (ReportData, error) are local
  reads. Empty report session selects all rows. Plan/report JSON matches baseline.
- NewLedger(root string) *Ledger; Read(context.Context) (History, error).
- Admit(context.Context, Request, Config) (Admission, error), where Admission
  contains Plan and Record *Record. A nil Record with no error means blocked.
  Initial read-only blockers precede lock infrastructure; locked re-admission
  remains necessary after that check. Domain results are independent snapshots.
- PublishPID(context.Context, id string, pid int) (Record, error).
- Finalize(context.Context, id string, Completion, Config) (Record, error).
  Completion holds Outcome string, ExitCode *int, ElapsedSeconds float64,
  ProcessPID *int, ExitConfirmed/OwnershipUnconfirmed bool, Tokens
  map[string]*big.Int, EstimatedUSD *json.Number, Complete/Failed bool and
  Source string. No arbitrary warning or raw error input. Rebuild post-cost
  planning identity from the fresh row. Preserve previously recorded warnings.
- PublicationError carries RunID string, ProcessPID *int and MayHaveCommitted
  bool with a fixed diagnostic. Callers must stop and inspect after this error;
  it is not authorization to retry or launch.

The existing lossless JSON decoder may be factored into internal/jsonvalue with
usage wrappers retaining the existing permissive stream contract. Budget adds
its strict finite/scalar/schema validation; do not fork permissive JSON semantics
or change usage's malformed-event behavior. No dependency version change is needed.
Planning/accounting retain finite float seconds; conversion to a representable
native execution duration is a future controller integration check.

Numeric provenance: [CPython 3.12.0 builtin sum implementation](https://github.com/python/cpython/blob/v3.12.0/Python/bltinmodule.c),
fetched 2026-09-21 UTC. Float totals need the runtime's accumulation semantics,
not merely the same JSON number spellings. This reference does not widen the
qualified interpreter versions or require Python in the installed Go runtime.

Finalize PID reconciliation: a nil Completion.ProcessPID preserves the current
recorded PID. A supplied PID must be positive and equal to an existing PID;
conflicts refuse without mutation. If the record's PID is missing/null, the
owning controller may supply the locally retained child PID after a failed
PublishPID operation, so uncertain ownership can still be recorded. This does
not authorize prompt delivery after failed publication. Finalize cannot change
owner_pid or replace an already published process identity.

For retained uncertain ownership, preserve outcome timeout only when the
supplied outcome is timeout; map every other valid terminal outcome to
launch_error. Never publish active interrupted/output_limit/failed/completed
rows, which the existing Python schema rejects. Reject unknown outcome strings.

Completion consistency: resolve the effective PID from supplied and stored
identity before deciding ownership. ExitConfirmed with a nil ExitCode is invalid;
refuse without changing the reservation. With an effective PID, either missing
exit confirmation, missing exit code or OwnershipUnconfirmed retains ownership.
Uncertainty without any effective PID cannot be represented as a schema-1
non-running active row: refuse and preserve the current reservation. A no-PID
launch_error with no uncertainty may finalize a pre-spawn failure without an
exit code or leader-confirmation claim. A launch_error always clears token/cost
metadata and completeness, regardless of the supplied accounting fields.

Schema clarification: active timeout/launch_error rows must contain explicit
null tokens and estimated_usd fields, matching baseline direct field access.
Their absence is not equivalent to null. Preserve permitted absent optional
fields on other valid rows. Planning only aggregates estimated cost when the
estimated-USD threshold is configured; irrelevant cost overflow cannot block
an otherwise permitted attempt with cost controls disabled.
