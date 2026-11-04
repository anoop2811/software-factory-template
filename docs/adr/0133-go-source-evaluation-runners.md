# ADR 0133: native source evaluation runners

Status: accepted design; implementation and qualification evidence are separate

Date: 2026-11-03 UTC

## Scope

Route the five shipped `eval/runners/*.sh` entrypoints to the existing
`factory eval run` owner. This is the next bounded R3 function boundary after
PR #163. Reuse the runner implementation and process supervision established in
docs/adr/0099-native-factory-orchestration.md:115; do not introduce a second
executor, change harness arguments, add dependencies or alter CI scheduling.
The stopped performance PR #162 is not a dependency.

## Invocation contract

Retain each public executable path as a thin adapter. Resolve its physical
source owner two directories above the runner, preserve the caller's working
directory, and exec that owner's `factory eval run HARNESS` with literal
original operands, stdin, environment, output streams and exit status. Preserve
direct and explicit `/bin/sh` invocation. Directory aliases and owner paths
ending in newlines retain their identities. Refuse a final-file symlink and fail
when the owning launcher is missing or fails; never use PATH, an environment
override or a historical runner as an alternate factory owner.

For a valid work directory, retain the mock's pass/fail/cheat modes and the
three real harnesses' existing argument order, optional `FACTORY_EVAL_MODEL`,
literal prompt and combined per-harness log. Ordinary harness nonzero exit
continues to return success to the evaluator so its oracle determines the score.
Missing inputs, cancellation and uncertain process ownership remain failures.
The example runner remains deliberately unwired and never selects a model.
Keep useful customization guidance in the evaluation documentation instead of
leaving a second active shell executor in the example adapter.

The source adapters adopt the already shipped native runner's error contract:
missing operands or an unwired harness return 2; an invalid work directory,
unreadable task or unavailable CLI returns 1. This explicitly replaces legacy
edge behavior: missing real-runner operands and the example previously returned
1, the mock could report success after a failed `cd`, and a failed task read
could still invoke a real CLI with an empty prompt. Do not reproduce those
fail-open behaviors. Qualification must expose these differences, including
proof that rejected inputs launch no CLI. This records a reviewable source
compatibility difference, not a claim of byte-for-byte legacy error parity.
The direct native runner's argument/error contract remains unchanged.

## Evidence and custody

An independent test author must observe the old scripts fail the new owner
boundary before implementation. Exercise every adapter against an owned
recorder for exact transport and failure propagation. Then execute real native
mock and fake-CLI runs, including a private source compile, relative work paths,
model/prompt/log behavior, example refusal, and golden pass/fail/cheat scoring
through the converted mock. No credentials or paid calls belong in these tests.

Reuse existing native runner acceptance for the full execution contract.
Historical fixtures must keep explicit frozen/test-owned runner bodies and
must not accidentally call the new source adapter without its owning runtime.
Retain all existing assertions and independently discover any updated test
census; do not change workflow topology, workers or timeouts. Tests compile only
in private fixtures and must preserve the operator's primary checkout binary.

Run affected source/native/installation checks, quality gates and the review
diamond. Record actual RED/GREEN results separately from design acceptance.
Hosted checks, review replies and merge are required before this boundary is
marked complete. The remaining R3 inventory and release gates remain separate.

## Nested runner shutdown

Independent correctness review reproduced a fake harness continuing after a
one-second golden timeout through a converted adapter. The adapter enters a Go
controller that owns its harness in a separate process group. Killing only the
outer runner group prevents that controller from cleaning up its child. Changed
adapter bytes also intentionally cease matching the frozen built-in runner
digests; neither default selection nor explicit `--runner` may bypass the user's
chosen executable merely to avoid this problem.

Use the existing `native.ExecuteCommandCooperative` for the golden evaluator's
external runner boundary. Its bounded termination grace lets a native controller
settle its separately grouped children before forced shutdown. Preserve the
acknowledged timeout result, oracle refusal, no-retry policy and final ownership
checks; do not increase the task's execution allowance or introduce a new
supervisor. Test timeout and parent cancellation with an actual fake harness
start handshake and observed process cleanup, including explicit and named
runner selection. A runner that ignores graceful termination must still be
forcibly bounded. All test children must be owned and cleaned up on failure.

Cooperative ownership is stricter than observing only the outer process group:
an external runner terminated by a signal, without acknowledging shutdown,
returns an ownership error even if its direct group disappears. Golden evaluation
must return nonzero and publish no score for that uncertain run. Record this in
EX-002 rather than silently retaining the earlier timeout score of zero and
successful evaluator status. A runner that acknowledges cleanup still produces
the established zero score on timeout. Preserve that regression with a controlled
acknowledging runner, and separately prove non-acknowledging/TERM-ignoring runners
are forcibly bounded, fail closed and cannot run the oracle.

Review also reproduced a separately grouped child still using the private task
directory after an unacknowledged runner was killed. Preserve that directory
whenever runner or oracle ownership is unconfirmed, including cancellation;
determine retention before returning a context error. Do not remove inputs still
possibly in use. Report only the generated retained path, safely quoted, alongside
the ordinary sanitized failure diagnostic; do not expose arbitrary child errors
or task contents. A small private typed error may carry that path to rendering.
Known-complete paths keep their existing automatic cleanup. Test uncertain
retention and the diagnostic independently, then explicitly clean up fixture
work only after observing its owned processes stopped.

## Nested runner result propagation

Security review reproduced an inner runner reporting uncertain ownership as
ordinary status 1 while its parent scored the oracle and removed live inputs.
Normal process exit is insufficient when a nested factory controller explicitly
could not establish cleanup. Carry that result across the process boundary.

For each external evaluation attempt, generate a fresh unpredictable private
token and pass it in `FACTORY_EVAL_OWNERSHIP_TOKEN`. Native `eval run` reports
versioned stderr receipts bound to that token, distinguishing completion,
execution rejection and ownership uncertainty. The parent recognizes only an
exact complete line `factory-eval-result-v1 TOKEN STATUS`, where TOKEN is 64
lowercase hexadecimal characters. Statuses include `failed`, `ownership-unconfirmed`,
and the handoff states defined below. Nested rejection cannot run the oracle; nested
uncertainty additionally causes a fatal error and retains the task directory.
Direct runner statuses stay unchanged. Custom runner nonzero exits, including
125, still receive ordinary oracle scoring without a matching receipt. Do not
reserve an ordinary exit code or classify scripts by name/body to guess ownership.

Validate the private token before rendering it and remove it from the environment
given to real harness CLIs. Do not disclose a receipt or arbitrary child errors
in public evaluation output. This is a narrow failure-reporting protocol, not a
replacement executor or an authentication boundary for arbitrary custom runners:
a selected custom runner is already executable code and can deliberately fail.
Tests must reproduce a detached pipe-holding fake CLI, observe live uncertain
work retained with no oracle or result, and clean up its owned process afterward.
Retain custom nonzero behavior, reject stale/malformed receipt matches, and prove
the harness never receives the private token. Use Go fixtures, without adding a
Python runtime dependency.

The source bootstrap is also part of this reporting boundary: it can reject or
lose compiler ownership before reaching native `eval run`. When that command is
requested with a valid private token, the launcher must emit the same receipt on
pre-exec failure, including early toolchain refusal and ordinary compiler errors.
Existing source ownership uncertainty must report `ownership-unconfirmed` and
retain its existing compiler/build custody. Do not reinterpret exit 125 globally.
Keep original direct-launcher statuses, successful exec, literal transport and
signal cleanup unchanged. A preliminary `failed` receipt may be followed by an
uncertainty receipt if cleanup discovers more; uncertainty always wins. Source
bootstrap reporting is lightweight shell transport because the Go runtime may
not yet exist. Qualify natural compiler failure and injected observation
uncertainty through the real source runner/evaluator boundary without a timeout.

## Bare shell operands

Some shells locate `sh mock.sh` through PATH but leave `$0` as `mock.sh`.
Without a caller-local regular readable script, that name does not establish
the runner's owner. Refuse it before parent traversal rather than selecting a
factory relative to the caller or repeating a potentially different PATH search.
Use an explicit script path for this form. Local `sh mock.sh` from the runner
directory and ordinary direct PATH execution remain supported; the existing
final-file symlink refusal still applies. Qualification must put an unrelated
factory above the foreign caller and prove that none of the five adapters
executes it.

## Compiler cleanup under nested supervision

The source compiler is separately grouped from its controller. Its existing
nominal half-second TERM grace must leave time for force and reap within the
outer supervisor's existing two-second cooperative grace. Repeated subshell and
sleep launches add scheduling cost beyond their requested sleep durations.
Probe a present group with the Bash builtin without a subshell; capture a failed
probe separately to retain explicit absence versus observation uncertainty.
Poll at 100 ms for five TERM and ten KILL observations, preserving the nominal
phase allowances with fewer spawned sleeps. Keep admission, actual child reap,
signal statuses and retained-build uncertainty unchanged. The bounded source
fixture must exercise delayed cleanup scheduling as well as ordinary execution.
This correction adds no dependency and does not enlarge task or global grace
limits. As with any process supervision, scheduler stalls can still exhaust a
deadline; such exhaustion must remain an ownership error, never a successful run.

## Incomplete external receipt capture

An external controller can exit normally after reporting a nested ownership
failure. If captured output exceeds its bound, the parent's receipt channel
may be incomplete even when that direct process has exited. Treat an external
runner's `output_limit` result as uncertain nested ownership: return an error,
retain and identify its private task directory, and invoke no oracle or score
publication. A deliberately chatty controller must not cause live inputs to be
deleted merely by placing its uncertainty receipt beyond the capture limit.
Qualify this with a real detached-child witness and observed overflow. Preserve
the existing native direct-harness behavior and ordinary custom nonzero scoring;
only the incomplete external-controller boundary receives this stricter rule.

## Adapter refusals before bootstrap

The owning launcher may be absent, non-executable or have an unavailable
interpreter, so its own bootstrap receipt cannot cover every adapter failure.
Emit a private `handoff-pending` receipt before all adapter owner/path checks,
only for a valid per-attempt token. An EXIT trap is insufficient: failed `exec`
bypasses that trap in supported Bash versions. The owning native runner emits
`completed` only after its normal successful return decision. Keep the existing
`failed` and `ownership-unconfirmed` terminal receipts. No extra shell supervisor,
helper dependency or shell-specific exec option is needed.

Read matching complete receipt lines in order. `handoff-pending` opens a handoff;
`completed` closes the currently pending handoff, but cannot erase a previous
`failed` or `ownership-unconfirmed` result. Completion before a later pending
receipt does not acknowledge that later handoff. An unmatched pending receipt
means a failed attempt with no oracle. Explicit uncertainty and incomplete output
capture still retain the work and abort evaluation. This distinguishes the
reproduced pre-exec refusals without changing custody guarantees for arbitrary
custom executable code, which this private protocol does not authenticate.

Initialize native receipt state conservatively to `ownership-unconfirmed` until
a normal return decision is reached: a panic must not emit completion merely
because a named return value defaults to zero. Explicit argument refusals and
confirmed ordinary execution failures select `failed`; explicit successful
runner completion selects `completed`. Preserve untokened direct shell status,
streams and operands, invalid-token silence and ordinary custom-runner nonzero
scoring. Test all five adapters with missing, non-executable and broken-interpreter
owners, rejected adapter paths, valid/invalid tokens, successful native handoff
and ordered completion. No ordinary exit code is globally reserved, and no
fallback runtime may run.

Render the adapter receipt directly from its canonical private variable. Do not
copy the token to a shell temporary: a caller-exported variable with that name
would retain its export attribute and leak the token through an alias to the
real harness. Preserve that caller variable's original value and qualify this
case in the existing native token-removal criterion.
