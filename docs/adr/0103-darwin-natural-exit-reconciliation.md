# ADR 0103: reconcile an absent Darwin group after natural leader exit

- Status: private qualification; protected-path human review required before merge
- Date: 2026-10-18 UTC
- Decision: 102

## Observed problem

PR #129's retained macOS qualification report recorded an installed-proof failure
with `phase=signal signal_errno=1 reaped=true exit_confirmed=true drained=true`.
Its predecessor report did not expose the failed cleanup phase. Linux and one
complete macOS run passed; a passing rerun alone did not establish a correction.

A bounded independent Darwin probe started and reaped 10,000 owned children.
In 5,000 immediate-signal cases, signal zero and SIGKILL succeeded. In 5,000
pipe-EOF cases, both returned EPERM before Wait; Wait reported normal status zero,
then signal zero returned ESRCH. Every destructive signal preceded reap, excluding
PID reuse. This demonstrates a kernel condition, not the full factory failure.

Apple's pinned XNU source explains why EPERM alone cannot identify the cause:
`killpg1` excludes zombie members and returns EPERM when no eligible member is
counted. Group iteration can also skip members that disappear before lookup.
Both zero and destructive signals use that group iteration. Sources fetched
2026-10-18 UTC:

- https://github.com/apple-oss-distributions/xnu/blob/f6217f891ac0bb64f3d375211650a4c1ff8ca1ea/bsd/kern/kern_sig.c#L1675
- https://github.com/apple-oss-distributions/xnu/blob/f6217f891ac0bb64f3d375211650a4c1ff8ca1ea/bsd/kern/kern_proc.c#L4266

## Bounded refinement

Keep the group termination attempt. EPERM is never evidence of absence.
Only on Darwin, after an ordinary completion whose leader was already reaped
before the termination attempt, may an EPERM result proceed to the existing
terminal-group observation. Require a confirmed exit status, fully drained input
and output workers, and an explicit ESRCH observation before the existing shared
cleanup deadline expires. Without every condition, retain typed uncertainty.
The leader must have exited normally, not from a signal. The execution context
must remain live when admitting reconciliation and after observing group absence;
a late cancellation or execution deadline cannot authorize this exception.

This permits no retry or second execution, new signal, deadline extension, zombie
classification, process-table heuristic, PID-based exception or permission grant.
The group must actually be absent. A positive result does not change a nonzero
child exit into zero. Timeout, interruption, overflow, failed publication and
pre-reap termination retain the original signal-error refusal. Non-EPERM signal
errors and every non-Darwin platform retain the original rule.

Retain the generic OwnershipError text, typed PID and bounded safe diagnostics.
Record whether the leader had been reaped before signaling, so a failed
qualification identifies whether this refinement could apply. This supersedes
the unconditional EPERM refusal only for the exact positive terminal proof above;
it does not reinterpret unresolved permission errors as success.

## Independent acceptance criteria

Observe RED before implementing this refinement. A real normally exited/reaped
leader with an injected EPERM report must still require an independent ESRCH
observation and drained workers; preserve its actual exit status. A live owned
descendant must keep uncertainty until explicit absence, including a paired real
termination control. Refuse an EPERM report before reap, during cancellation,
with unfinished capture, with unknown exit status, or after the cleanup deadline.
Preserve non-EPERM error refusal and fixed diagnostic privacy controls. Do not
replace any regression with a weaker assertion merely to obtain green CI.

The new path requires independent full-source review and final-head Linux/macOS
qualification. A green run does not prove every possible Darwin race eliminated;
the stated safety result is the exact positive terminal proof, not a timing guess.

Exercise cancellation after the successful absence observation independently of
cancellation before admission. A private per-invocation process observer defaults
to the unchanged real group observer; the test wraps that observer, requires its
actual positive absence proof, then cancels the execution context before returning.
Removing the post-observation cancellation guard must make this control fail.
This introduces no external configuration or additional process authority.
