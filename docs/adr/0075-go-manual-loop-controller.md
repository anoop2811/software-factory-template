# ADR-0075: Go manual loop execution and resume

Status: accepted for private source qualification; installed activation pending.
Decision: 74. Date: 2026-09-25 UTC.

## Scope and oracle

Implement milestone three of the existing four-part loop package (ADR-0073),
without adding a new progress denominator. Preserve the manual behavior of
scripts/lib/loop.py at immutable 76952eaa63aebd1ecd282f5ab51dd7c3627cb497.
Use the qualified fingerprint, checkpoint, budget-history and native-supervision
components; do not duplicate process or filesystem lifecycle implementations.
Public option parsing, bounded paid loops and installed cleanup stay pending.
No model calls, native CLI probes, commits, pushes or releases occur in this slice.

## Private compiled boundary

With FACTORY_BRIDGE_PROTOCOL=1, accept exactly:

- factory loop manual plan HARNESS SESSION TASK
- factory loop manual run HARNESS SESSION TASK
- factory loop manual resume HARNESS SESSION TASK

HARNESS is codex, claude or opencode; identifiers follow budget rules. Literal
requests only; malformed/extra/flag-like requests fail before effects with a
sanitized diagnostic and status 2. Root is FACTORY_LOOP_ROOT or cwd, as with the
other private loop requests. Existing checkpoint status stays read-only.
Configuration comes from existing FACTORY_LOOP_* and FACTORY_BUDGET_* inputs.
No stdin or prompt file is read. Prompt identity is the existing digest of empty
string. Manual mode is allowed with both features disabled, as in the oracle.
All valid outputs are one JSON value plus newline, no raw check output.
Plan JSON matches manual make_plan fields and blocker order; exit 0/2 is based
on blockers. Plan validates a stable snapshot, but creates no state or process
other than bounded read-only Git/ERE probes. It must not probe a native client.
Run/resume produce a record after durable terminal publication (0 only for
manual_passed); a blocked plan may produce plan JSON with status 2.
Publication/encoding/output failure must not advertise success or retry a check.

## Admission and record lifecycle

Read and validate whole loop and budget histories before writes. Empty effective
check command, active budget records or any active/uncertain loop block execution.
Run refuses an existing session/task. Resume requires the same explicit harness,
manual mode, fresh source/policy/empty-prompt identity, manual_passed/manual_failed
outcome and remaining total allowance; preserve unknown fields, evidence, original
started_at, baseline, counters and reserved_seconds. Never reset budgets/time.
Hold the persistent nonblocking Python-compatible loop lock for the whole run.
Repeat admission and establish stable observations under that lock; freshness
assessment from ADR-0074 alone is never launch authorization. Recheck global
active budget state immediately before the check as in the Python controller.
This is cooperating-process parity, not atomic exclusion of arbitrary external
budget launches or a sandbox against an unrestricted writer.
Create schema-1 records with Python-compatible fields, starting status active,
phase starting, outcome running, current owner PID, null process PID, empty
arrays and zero attempts/no_progress. Persist before launching. Resume changes
owner/status without erasing history. Saves update cumulative elapsed time.
Hold consumed allowance across resumes; total deadline uses monotonic elapsed
for this invocation plus prior stored elapsed. Avoid time.Duration overflow for
accepted large finite configuration; cap conversion safely to representable time.

## Deterministic check and evidence

Before execution verify policy and stable source; baseline HEAD and safety inputs
must remain unchanged. Save phase check. Execute bash -c with the existing
`exec 2>&1\n` prefix, the configured command as one literal argument, empty stdin,
checkout cwd and FACTORY_AGENT_ROLE=reviewer. Resolve bash using the
request's captured environment/PATH, preserving cwd-relative lookup; execution
and fingerprint probes must observe the same captured environment. This shell evaluation is explicitly
intended for the user-configured check command, not interpolated agent input.
Reuse native process supervision, including process-group cleanup after leader
exit, bounded pipe capture/drain and PID publication callback. Publish process
ownership before continuing supervised input; this is not a pre-exec sandbox.
Allowance is the lesser of remaining total time and configured check timeout.
Do not launch if it is exhausted. Capture the existing 16 MiB bound including
combined stderr; retain no raw output in checkpoint or response.
After confirmed cleanup, clear process_pid; otherwise retain ownership and mark
uncertain. Append check evidence with exact command, exit_code (nullable), outcome,
duration_seconds, pre-check snapshot, harness and role deterministic. Persist,
then establish a new stable snapshot equal to the checked one before accepting
manual_passed/manual_failed. Any source, HEAD, safety or policy change hands off.
Manual success remains status stopped, outcome manual_passed, phase finished:
"Manual check passed; implementation and review were not performed."
Ordinary nonzero check similarly yields manual_failed and status 2:
"Manual check failed; no model was invoked."
Preserve Python next_action semantics and no model invocation/budget-run evidence.

## Failures and interruption

Timeout, output limit, failed launch, stale evidence and unconfirmed cleanup stop
with status 2 and a handoff; no automatic retry. Signals cancel owned processes. Reuse the native supervisor's five-second
cleanup bound, then give terminal persistence and final response their own shared
five-second context detached from cancellation. No fresh snapshot probes or
check launches occur once the parent is canceled; ordinary uncanceled freshness
probes retain the fingerprint component's existing bounds. Context deadlines
bound cooperative operations, not an arbitrary blocked filesystem syscall. Failed
ownership confirmation retains a PID and uncertainty. A reaped/certain failure
must not invent an unknown live child. This private qualification deliberately
uses observed ownership for signal interruption rather than Python's blanket
KeyboardInterrupt uncertainty. It still yields interrupted/status 2 and cannot
resume as a successful manual check; installed migration must reconcile this
explicit difference before activation. Checkpoint publication failure poisons the
transaction; do not attempt a second save through that transaction. Preserve the
last durable active record as the conservative recovery barrier and emit only a
sanitized error. Post-rename uncertainty never authorizes another process launch.
Existing records, unrelated extension fields, user changes and budget metadata
are preserved. This slice exposes no recovery/reset or arbitrary history editor.

## Qualification

Independent outside-in Ginkgo/Gomega RED before production. Compare actual CLI
manual plan/run/resume semantics with the immutable Python oracle, normalizing
only volatile PIDs/timestamps/durations and path-dependent identity. Cover all
three harness metadata values, no-model sentinels, pass/fail, repeated fresh
resume, duplicate run, stale source/safety/policy, budgets, corrupt state, locks,
unknown-field retention, timeout, output overflow, signals, descendant-held pipes
and publication failures. Private collaborator seams may inject lifecycle failures;
real process/filesystem tests establish their observable boundary. Full existing
budget/native/checkpoint regressions and race/quality gates must stay green.
No dependency version changes are needed. Authoritative API documentation read
2026-09-25 UTC: https://pkg.go.dev/os/exec#Cmd.Wait and
https://pkg.go.dev/context#WithoutCancel. Cancellation alone does not establish
process-group cleanup; preserve the already-qualified supervision contract.

## Validation-order qualification

Read and validate existing checkpoint history before evaluating configuration
that can execute ERE probes. A corrupt checkpoint must refuse without running
those probes or creating execution state. This makes the existing immutable-oracle
ordering explicit; the independent compiled-CLI regression already covers it.
