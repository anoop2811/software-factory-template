# ADR-0070: Composed Go budget execution controller

Status: accepted for implementation under specs/001-go-runtime-conversion.md.

## Scope and reuse

Decision 69 continues the budget-controller work package by composing the Go
ledger, native supervisor and usage parser. The immutable Python behavior source
is commit 4cc771e894e11d5024032106aeb0ef788997bd29, scripts/lib/budget.py:472.
Keep public commands, shell configuration/model routing, output formatting,
loop checkpoints, installed activation and predecessor retirement for their
existing compatibility/delivery gates. This is an internal controller candidate,
not another completed acceptance package or an installed runtime switch.
In particular, live plan-before-model output and JSON/text rendering parity are
deferred; returning a structured result does not qualify those public contracts.

Use a separate Runner within internal/budget; Ledger retains storage ownership.
Reuse Ledger admission/publication/finalization, native Prepare/Preflight/Execute
and usage Parse/Response. Do not copy schema validation, process supervision,
role policy, JSON parsing or completion projection. No JSON serialization to
recover private record fields. Shared admission internals may accept a private
deadline policy; the existing exported Admit contract stays unchanged.

Prefer concrete types and small per-instance collaborators at actual fault
boundaries. No exported dependency-injection framework, mutable global hooks,
new environment fault switches, or speculative interface hierarchy. Context is
a method parameter, never retained in Runner. No new dependency is needed.
Existing Cobra and Ginkgo/Gomega remain the CLI/test choices. Any future added
library must have a verified permissive open-source license (including relevant
transitive dependencies), a concrete reuse benefit, and current authoritative
API/version evidence before pinning; popularity alone is insufficient.

## API and ordering

- NewRunner(root string) *Runner.
- RunInput: PromptFile string, Prompt *string, Environment map[string]string,
  WantResponse bool. A nonnil Prompt overrides PromptFile, including empty text.
- RunResult: Plan Plan, Record *Record, Response string with json:"-", ExitCode int.
- (*Runner).Run(context.Context, Request, Config, RunInput) (RunResult, error).

Initial read-only validation/plan precedes prompt loading, local CLI probes and
storage creation. Blocked plans return code 2, nil Record and no error. Validate
positive finite execution seconds representable as time.Duration before
reservation or native probing; refuse overflow and subnanosecond values rather
than wrapping or rounding a zero allowance into launch. Do not narrow the ledger's
standalone finite-float planning domain.

Load a regular prompt file only after initial admission, bounded to 16 MiB,
resolved relative to caller cwd, scalar UTF-8 with no NUL and universal newline
conversion matching Python text
loading. Refuse special files without blocking. In-memory prompts go through the
same content bounds, retaining literal newlines (only file loading normalizes
newlines); native Prepare enforces its prefixed stdin bound. Errors
must not expose prompt contents, paths, native output or injected error details.
RunInput.Environment is preparation overlay input, not a replacement process
environment; Execute retains its existing inherited-environment contract.
Prepare literal native arguments/role environment, then run local Preflight.
No paid/native execution before durable locked admission. Failed preflight must
leave history/storage untouched; preserve typed uncertain probe ownership.

If the parent has a deadline, clamp timeout after preflight and recompute it
inside the shared ledger lock after waiting, using the remaining monotonic
allowance. Never increase caller limits. Check cancellation/deadline before
publication and before spawn. Reserve min(timeout, remaining session time),
refusing an unrepresentable/subnanosecond effective duration before a write.
A last-slot race must admit exactly one process and retain all prior accounting.

Use the exact reservation's ID and duration for Execute. The onSpawn callback
captures the PID locally before calling PublishPID, which must finish before
prompt bytes are delivered. Never retry admission, PID publication or execution
on error. Preserve ambiguous PublicationError identity through the returned
error chain. Admission errors do not authorize finalization or launch.

## Completion, cancellation and privacy

After admitted execution, attempt finalization exactly once using a fresh bounded
cleanup context that retains context values but is detached from cancellation.
Use a five-second cleanup budget, including usage/answer selection and final
ledger lock waiting; cancel its timer. Native supervisor cleanup has its own
existing five-second bound. No unbounded retry or Background-only finalization.
Filesystem qualification remains the local cooperating POSIX namespace from
ADR-0069; a context cannot interrupt an arbitrary blocked filesystem syscall.

Retain the local child PID even if publication failed. PID-publication failure
stops stdin and terminates/reaps through native Execute, then may finalize that
same reservation once with observed exit evidence. It is cleanup, not a retry.
Keep the original publication error observable even if cleanup succeeds. If
ownership is unconfirmed, preserve active schema-1 uncertainty through Finalize;
a leader exit alone must not release the reservation. Missing/disappeared/foreign
rows must never be recreated. Finalization failure returns no completed Record
or answer, preserves the ledger's existing evidence, and cannot report success.

Use the shared stream parser for ordinary completed/failed execution. Map
metadata incompleteness/failure using the existing Finalize rules. Timeout,
interruption, output-limit and launch errors clear claims; do not feed the
executor's overflow sentinel (16 MiB + 1) into the smaller accounting parser and
accidentally replace output_limit with launch_error. Parsing errors become
launch_error with known ownership retained. No raw output enters history/results.

Extract an answer only when requested, native execution and metadata qualify for
completed success, and ownership is confirmed. An answer-selection error becomes
launch_error; no answer escapes. Persist final metadata successfully before
returning any answer. Response is excluded from default result JSON; a future
human-output/loop adapter may explicitly consume it. Error diagnostics are fixed;
retain typed underlying ownership/publication errors without printing their data.

Ordinary outcomes have nil error and codes completed=0, timeout=124,
interrupted=130, otherwise=1. Pre-admission errors use code 2. Any post-admission
error returns a nonzero code (normally 1), even when cleanup publishes a final
record. A canceled-before-spawn admitted run uses a schema-valid pre-spawn
launch_error record rather than inventing PID/exit evidence. Finalization is not
authorization for another run after an error.

## Independent acceptance

Outside-in Ginkgo/Gomega uses temporary compiled drivers and fake native CLIs;
no installed routes or paid calls. Observe behavioral RED before implementation.
Cover each harness's exact arguments/roles/model, disabled and blocked no-effect
paths, successful history/answer, incomplete metadata, native nonzero exit,
timeout/cancellation/overflow, preflight refusal, prompt bounds, deadline expiry
while waiting for lock, simultaneous admissions, and private metadata output.
Use per-instance internal collaborators for deterministic publication, parser,
answer, uncertain-exit and finalization failures. Exercise real filesystem and
process boundaries where possible. Independently review correctness, security
and test claims, verify findings before accepting them, and run the source gate.

## Authoritative references

Fetched 2026-09-22 UTC: https://pkg.go.dev/context#WithoutCancel documents detached
cancellation; https://pkg.go.dev/context#WithTimeout defines explicit bounded
cleanup; https://pkg.go.dev/time#Duration defines the signed nanosecond duration.
Existing library license sources: https://raw.githubusercontent.com/spf13/cobra/v1.10.2/LICENSE.txt
(Apache-2.0), https://raw.githubusercontent.com/onsi/ginkgo/v2.32.1/LICENSE and
https://raw.githubusercontent.com/onsi/gomega/v1.43.0/LICENSE (MIT). No versions change.

## Existing timeout uncertainty contract

A native timeout with unconfirmed ownership and an OwnershipError must retain
the timeout outcome in its active reservation, as required by ADR-0069. The
controller still returns an error/nonzero code and no answer; preserving the
recorded timeout does not claim completion. Other execution/publication errors
continue to map to launch_error unless the native timeout evidence is retained.

## Cooperating atomic snapshot replacement

An unlocked preliminary Ledger.Read can overlap another cooperating controller's
atomic history publication. A proven replacement between pathname inspection
and descriptor inspection is not corruption. Permit at most eight attempts to
acquire a validated read-only snapshot, checking cancellation on each attempt.
Retry only a private replacement condition established from distinct regular
single-link history inodes, or an originally validated regular descriptor that
was unlinked by replacement with a different regular single-link pathname entry.
Keep descriptor size/modtime stability, scalar/schema bounds, pinned directories
and symlink/hardlink refusal. No parse, open, ownership, permissions, generic
storage error, admission or publication retry is permitted. Exhausted snapshot
attempts fail closed. Locked reads retain strict checks; this policy is only for
the read-only snapshot used before locked re-admission or reporting. A small
private per-instance history-open collaborator may deterministically exercise
replacement windows in tests without an environment switch or global hook.

## Persistent lock first creation

Concurrent ordinary O_CREATE opens of the initially absent lock returned ENOENT
in the local Darwin Go toolchain experiment on 2026-09-22 UTC, independently of
ledger/history operations. This is an observed boundary behavior, not a claim
about the precise operating-system or Go defect. Avoid relying on that creation
pattern: attempt O_RDWR|O_CREATE|O_EXCL|O_NOFOLLOW|O_NONBLOCK once; only ErrExist
permits one open of the existing pathname with O_RDWR|O_NOFOLLOW|O_NONBLOCK and
without creation/truncation flags. Other errors fail immediately. If the existing
lock disappears before that second open, refuse; never recreate/unlink it.

Both paths retain the existing descriptor ownership, regular/single-link,
pathname identity, chmod and flock checks. This handles initial file acquisition,
not a retry of admission or publication. Keep all subsequent lock ownership and
safety checks; do not skip history validation to mask an unproven failure. A
private per-instance lock-open collaborator can test the ErrExist-only branch
and error/identity refusal. Verify real competing controllers repeatedly with
ordinary binaries as well as race-instrumented binaries before qualification.

Reference fetched 2026-09-22 UTC: https://pkg.go.dev/os#Root.OpenFile and
https://pkg.go.dev/os#pkg-constants define file opening and exclusive creation.


## Review clarification: downgraded completion parsing

A completion downgraded to launch_error because it lacks a process identity must
skip usage parsing and answer extraction, even if a private test collaborator
returns an inconsistent native completed/failed outcome. Gate processing on the
normalized completion outcome and a present PID. Ledger.Finalize independently
clears launch-error claims; retain that defense. The concrete native executor
always records a spawned PID and returns ordinary confirmed interruption as an
outcome with nil error. Do not broaden timeout uncertainty handling to claim a
confirmed interruption when process ownership is unresolved.


## Cleanup timing regression boundary

Measure the finalization lock-wait regression from the collaborator's acquired
lock immediately before returning execution, rather than including unrelated
admission/preflight filesystem work. Retain a finite upper bound and the
five-second lower-bound check, plus error/no-answer/no-final-record assertions.
This test clarification does not change the production cleanup deadline.


## Overflow fixture lifecycle

The overflow fixture must stay alive after writing limit+1 bytes until the
supervisor terminates it, under the existing external test watchdog. Immediate
exit races group signaling with an unreaped child on Darwin: an isolated local
probe observed EPERM before reap and ESRCH after reap. This is consistent with
the macOS CI ownership-uncertainty failure, but that CI run did not record the
underlying syscall error. Preserve production fail-closed signaling semantics
and the existing signal-failure regressions. Do not suppress EPERM or accept an
active launch_error as a successful overflow test result.


## Admission duration regression boundary

The reservation duration is computed after lock acquisition, before durable
publication. Measure its deadline bound against a monotonic timestamp recorded
before the competing lock is released; later fsync time must not make a valid
reservation fail the test. Execution retains the original parent deadline.
The test must join its unlocking worker before closing the shared descriptor,
including assertion-failure paths. A delayed-publication fixture must still
prove lock-wait deduction and preservation of the execution context deadline.
