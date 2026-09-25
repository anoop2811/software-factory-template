# ADR-0074: Go loop checkpoint storage and recovery invariants

Status: accepted for source implementation under specs/001-go-runtime-conversion.md.

## Scope and source

Decision 73 implements milestone two of the existing four-part loop conversion
package: checkpoint storage and recovery invariants. The immutable oracle is
scripts/lib/loop.py at 76952eaa63aebd1ecd282f5ab51dd7c3627cb497: Store starts
at line 197; locked at 266; write at 282; status/resume admission at 564-615.
docs/LOOPS.md remains the governing recovery contract. Installed commands,
manual execution/resume, bounded iteration and migration/retirement remain open.

This slice does not recover uncertain processes or manufacture resume permission.
It never launches checks, native CLIs or models, deletes history, resets counters,
clears uncertainty or rewrites fingerprints. A dead owner PID alone proves nothing
about children. Recovery remains an explicit human action after process inspection.

## History identity

Keep .factory/loops.json schema 1 and the persistent .factory/loops.lock inode.
Missing history reads as {"schema":1,"runs":[]}; read/status must not create the
directory, lock or data, chmod existing paths, or parse unrelated configuration.
Validate the whole history before returning any selected record. Preserve unknown
top-level/run fields, array order, arbitrary evidence/budget_runs contents and
exact integer values. JSON object order and numeric spelling may normalize; types
and values may not. Duplicate JSON keys retain the baseline's last-value behavior.

Match Store.validate's field contract, not a stronger invented domain schema:
session/task use budget identifiers and their pair is unique; harness is codex,
claude or opencode; mode manual/bounded; status active/stopped/completed; evidence
and budget_runs are arrays. attempts/no_progress are nonnegative exact integers
with finite float conversion; elapsed_seconds/reserved_seconds are nonnegative
finite numbers. policy/prompt/started_at/phase/outcome/stop_reason/next_action
are strings. snapshot/baseline have exactly head/source/safety string fields,
without imposing hash syntax. owner_pid is an arbitrary-size positive integer;
process_pid is absent/null or an arbitrary-size positive integer. uncertain is
boolean. Booleans and integral floats are not integer fields.

Use existing jsonvalue decoding and Python canonical encoding, preserving escaped
lone-surrogate strings and unknown numeric identity. Require valid UTF-8 JSON
input, depth at most 512, the existing decoder's 4300-digit integer limit, and
actual input/encoded output at most 32 MiB.
Nonfinite numbers anywhere are refused before a rewrite, including unknown data.
This is an explicit private fail-closed qualification beyond Python's permissive
unknown-field reads; no destructive repair follows from refusal. Unknown integers
need not fit float64. Keep the budget parser's existing stricter scalar contract.

## Shared storage and publication

Extract the established budget filesystem mechanics into a concrete internal
statefile component parameterized by fixed history/lock/temp names. Keep domain
parsing and publication metadata in the budget/loop wrappers. Preserve existing
budget error contracts, transaction timing and private test seams. Do not create
parallel copies of locking, descriptor validation or atomic publication logic.

Reuse pinned .factory directory handles; nofollow/nonblock regular descriptors;
single-link checks; mutation ownership checks; 0700 directory and 0600 lock/data;
bounded reads; and persistent exclusive-first lock creation with an EEXIST-only
existing-file retry. A vanished/replaced held lock or directory refuses further
work. Refuse unsafe symlink, hardlink, FIFO and nonregular storage paths without
following or deleting them. A source root selected by the caller is permitted.
These are cooperating-process invariants, not a hostile same-user sandbox.

Budget locks retain bounded context-aware waiting. Loop locks refuse contention
immediately, using the same fcntl.flock namespace as Python. A concrete loop
transaction owns the lock until Close; it can span future controller work.
Only a held, open transaction may publish after reading its current snapshot;
Close is idempotent and further transaction operations refuse safely. Successful
publication advances the expected history inode so multiple controller saves in
one transaction remain possible. An uncertain publication disables further writes
in that transaction and requires inspection, not an automatic retry.
Lock acquisition is not evidence of checkpoint eligibility. Reread under the
lock; do not trust an earlier read. Read-only acquisition may retry at most eight
proven safe atomic replacements, never parse errors or arbitrary failures;
locked reads do not retry. Preserve budget replacement behavior unchanged.

Validate and encode before publication; use an exclusive private temporary file,
complete write, file fsync, rename, then directory fsync. Recheck directory,
held lock and prior history identity before rename. Clean up only the owned
temporary inode. Reject encoded output over 32 MiB before creating a temp file.
Publication errors distinguish whether rename may already have committed; never
silently retry, overwrite the new state with old data, or report success after
an uncertain fsync/cleanup. Preserve the original checkpoint on pre-rename errors.
Context cancellation stops work without abandoning owned descriptors or temp data.

## Read-only resume assessment

Provide a pure assessment over validated loop and budget histories plus an
explicit request (session/task, harness/mode, current snapshot, policy digest,
prompt digest, positive finite timeout). The caller supplies observations; this
helper does not itself prove that the supplied fingerprints describe current
files. Future controllers must gather fresh stable observations and repeat
admission under their locks before any launch.

Refuse any active budget run, or any active/uncertain loop record, across the
whole checkout. Require the selected identity and original harness/mode. Require
exact snapshot, policy and prompt equality. Only manual_passed/manual_failed
manual records are eligible; terminal bounded runs never restart automatically.
A completed manual record is not independently forbidden by the baseline:
eligibility follows the actual field predicates, not a new stopped-only rule.
Consumed elapsed time must be strictly below timeout. Compare arbitrary-size
integer elapsed values to float timeouts without lossy rounding; subtract using
baseline Python float semantics only after admission. Return the untouched record
and remaining allowance. Never mutate status, PID, counters, evidence or time.
Missing, malformed, stale or uncertain records remain available for inspection.

## Private compiled qualification boundary

Under FACTORY_BRIDGE_PROTOCOL=1, add literal requests:

- factory loop checkpoint read: one complete JSON history.
- factory loop checkpoint status SESSION TASK: one selected record.
- factory loop checkpoint roundtrip: acquire the nonblocking loop transaction,
  reread and durably rewrite exactly that validated history, then emit it. This
  private qualification operation does not accept replacement records from stdin.
- factory loop checkpoint resume-check SESSION TASK: consume one bounded JSON
  object from stdin with exactly harness, mode, snapshot, policy, prompt and
  timeout_seconds; emit {"eligible":true,"remaining_seconds":...,"record":...}
  only when the read-only assessment succeeds. Read budget history without
  creating it. This operation is an assessment, never a launch authorization.

Use FACTORY_LOOP_ROOT or caller cwd, as the fingerprint boundary does. Reject
unknown/extra operands and invalid identifiers with sanitized diagnostics before
storage access. No flags, help or completion through the private protocol. Success
is 0; errors are 2, sanitized stderr and no usable partial JSON on pre-output
failure. Retain scoped SIGINT/SIGTERM context handling and complete cleanup before
returning. Existing fingerprint and budget private requests remain compatible.
Cancellation must interrupt a blocked command-owned stdin read; checking context
only between reads is insufficient. Close command-owned input on cancellation,
following the existing review client's bounded input-lifecycle pattern. Do not
claim deadlines for arbitrary filesystem syscalls.

An independently observed inherited-pipe case shows that closing a nonpollable
os.Stdin can itself wait for the read. For pipe/socket input, duplicate the
descriptor, temporarily enable nonblocking mode and construct a pollable os.File
before reading; verify deadline support and close that owned duplicate on
cancellation. Preserve and restore the original descriptor flags on every exit,
including setup failure, because dup shares the underlying file description.
Do not leak a blocked reader goroutine, close the caller's original descriptor,
or leave its nonblocking mode changed. Treat flag-restoration failure as an error.
Regular files retain bounded synchronous reads. Other supplied reader types need
an explicitly owned closeable lifecycle to qualify blocked-read cancellation.
The already pinned BSD-3-Clause golang.org/x/sys module may become a direct
dependency for its Unix descriptor helpers; no module version changes.

## Independent qualification

An independent evaluator first observes compiled-CLI RED before implementation.
Ginkgo/Gomega fixtures compare read/status/roundtrip with immutable Python Store,
including unknown fields, large numbers, surrogate identity, malformed data and
actual bounds. Exercise all resume predicates without changing stored bytes.
Use real Python/Go locks in both acquisition orders and prove lock inode retention,
nonblocking refusal, private modes and no writes for read paths. Use private
collaborator seams for file/directory fsync, rename, replacement and cancellation
faults, with observed old/new bytes and publication uncertainty. Re-run existing
budget race/acceptance and full source quality checks after shared extraction.
No new dependency modules or toolchain pins; no paid calls or public activation.

## Sources checked

Fetched 2026-09-24 UTC: https://pkg.go.dev/os#Root (rooted file operations),
https://docs.python.org/3/library/fcntl.html#fcntl.flock (nonblocking flock), and
https://docs.python.org/3/library/json.html (JSON types and encoding). Existing
repository source, not a new dependency, supplies the shared implementation.
Input lifecycle APIs checked at https://pkg.go.dev/os#NewFile and
https://pkg.go.dev/golang.org/x/sys/unix#FcntlInt; the pinned module's local LICENSE
was inspected and is BSD-3-Clause.

## Nonpollable device input follow-up

Review reproduction on 2026-09-25 UTC observed the private resume-check command
remaining blocked after SIGTERM when stdin was a terminal device. The pipe/socket
adapter does not qualify character or block device cancellation. Before parsing,
refuse nonregular file descriptors other than supported pipes/sockets with the
existing sanitized input-preparation error. Do not close or change the caller's
original descriptor. Regular files retain the stated synchronous filesystem
limitations; supplied reader ownership rules remain unchanged. This private
source boundary must not silently accept an unqualified device and hang.
Independent refusal regressions precede the production correction.
