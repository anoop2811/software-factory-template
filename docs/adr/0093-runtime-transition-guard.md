# ADR 0093: guard runtime transitions across cooperating controllers

- Status: accepted implementation refinement of the approved Go conversion spec
- Date: 2026-10-03 UTC
- Decision: 92

## Context

The Go and legacy budget controllers probe the native CLI before budget
admission. A paused preflight therefore need not appear in the ledger or hold
its lock. Loop controllers additionally run local snapshot commands and checks.
Acquiring existing accounting, checkpoint or recovery locks does not establish
that these processes have stopped. The required exclusion boundary covers
preflight through terminal publication, including uncertain child ownership.
See specs/001-go-runtime-conversion.md:132 and
specs/001-go-runtime-conversion.md:313.

## Decision

Introduce a checkout-local interoperable transition protocol for cooperating
Go and legacy Python budget and loop controllers. It is an installation safety
component, not activation, restoration or authorization to retire any script.
Reuse existing dependencies; do not change model invocation or budget defaults.

### Persistent exclusion

Use the single permanent `.factory/runtime-transition.lock` inode. Explicit
work takes a nonblocking shared `flock`; a future migration takes a nonblocking
exclusive lock. Never unlink, truncate, replace or repair this lock. Shared
access preserves the existing accounting concurrency limits and permits a
bounded loop's nested budget invocation without transferring authority through
environment variables. Contention refuses before probes or checks start.

Validate descriptor and pathname identity, ownership, regular type, link count,
empty content and exact private mode. Pin the project and state directories;
reject links in controlled checkout ancestry and replacement rather than following or fixing
them. Create absent private infrastructure exclusively and read it back. Do not
change permissions on pre-existing unsafe control files or directories.
The activity directory itself must be 0700. A pre-existing owned `.factory`
parent without group/other write permission is compatible, including 0755;
the guard preserves its mode rather than repairing it.
Existing accounting/checkpoint stores may still tighten their parent to 0700
under their established privacy contract. Guard revalidation accepts that safe
change without accepting a different inode or group/other write permission.
The selected project root must itself be a physical directory, pinned by its
descriptor and original pathname; reject a final-root symlink and traversal.
Stable system path aliases above that boundary, such as macOS `/var`, remain
compatible. All controlled children are opened relative to pinned parents with
no link following, and original project pathname identity is rechecked.

### Durable activity

After acquiring shared exclusion, exclusively create an unpredictable empty
0600 regular marker in `.factory/runtime-activity/` (0700). Sync the marker and
its directory and the newly created infrastructure before executing any child.
Here, execution children mean native preflight/invocation, execution snapshots
and deterministic checks. Existing read-only configuration validation, including
regex validation, remains before initial admission and does not create a guard.
The marker is deliberately not a PID lease: timeouts, parent death, an empty
ledger, reused PIDs and an unlocked inode cannot establish descendant exit.

Hold the guard and marker through preflight, admission, execution, process
cleanup, metadata processing and terminal publication. Budget controllers may
remove their own marker after a known harmless pre-admission refusal or a
durably completed record with confirmed ownership. Active reservations,
unconfirmed preflight/native ownership, uncertain completion and potentially
committed publication failures retain it. Loop controllers hold an outer guard
before snapshot subprocesses and checkpoint admission through terminal publication;
remove their marker only after a known refusal before any owned work or a
durably terminal checkpoint with no uncertain ownership. Errors after loop work
starts conservatively retain the marker.
Some loop errors are converted to a terminal handoff checkpoint without retaining
their original typed cause. Consequently only ordinary safe outcomes
(`manual_passed`, `manual_failed`, `approved`, `attempt_limit`, `no_progress`,
`repeated_failure`) may qualify cleanup after work starts, with terminal status
and no uncertain/process ownership. Handoff or interrupted outcomes retain the
marker; a terminal status by itself is not proof of harmless completion.

Removal checks the exact originally created descriptor/path identity, unlinks
only that marker and syncs the activity directory. Identity, close and sync
failures are reported; neither successful work nor cleanup permits removing
someone else's evidence. Abrupt process termination leaves the marker in place.
If syncing the directory fails after a qualified marker unlink, report the
failure without claiming durable removal or creating replacement evidence.
Owned work has already ended before that unlink; crash recovery may retain the
original marker. An incomplete or uncertain invocation never reaches unlink.

An exclusive guard must inspect the bounded activity directory while holding
the permanent lock and refuse on any entry, malformed evidence, unavailable
storage or unsafe identity. It must not delete stale markers, infer liveness or
automatically recover them. No public migration command is added in this slice.
The exclusive component is a prerequisite for the later transaction consumer.
Even successful acquisition excludes cooperating participants only; it cannot
authorize activation on an unbridged installation or erase ledger/checkpoint
ownership blockers. Explicit quiescence and reverse transaction ownership
remain separate requirements.

### Compatibility and packaging

Legacy native help currently uses `subprocess.run` without process-group
ownership. Waiting for the help leader alone does not establish descendant
exit, even on a successful help result. Correct that unsafe baseline boundary
under specs/001-go-runtime-conversion.md:333: run help in an owned session,
capture bounded output, and perform bounded group cleanup/reap before declaring
preflight harmless. Reuse the existing Codex probe cleanup mechanics where
practical. Unconfirmed group cleanup must retain a typed ownership error and
the activity marker. Preserve local help flags, channels and the ten-second
capability deadline; no provider/model invocation or new retry is allowed.
An independent real descendant regression must fail before this correction.
Generic legacy preflight failures without exit proof retain the marker rather
than claiming harmlessness from an empty ledger.

The same rule applies to execution snapshot helpers: a successful Git/grep
leader can leave same-group descendants with detached output streams. Independent
actual public manual-loop diagnostics reproduced this in both runtimes before
correction. Require outside-in RED, then shared supervised execution with
confirmed group cleanup on success as well as failure. Reuse Go's existing
native command supervision and Python's owned probe machinery where practical;
keep literal argv, cwd, environment, capture limits and deadlines. A cleanup
failure retains ownership evidence and the outer loop activity marker. Pure
read-only validation still does not create transition infrastructure.

Read-only plans/status/reports and disabled or initially blocked runs remain
read-only. No new model calls, retries, background services or network requests.
Existing output schemas, identifiers, accounting and loop evidence remain
unchanged. Guard refusals use existing safe command error channels.

The Python protocol shim is temporary coexistence machinery, shared by its
budget and loop controllers. Ship it in legacy init/upgrade and the Go init
source-asset inventory; it is not an active recovery fallback. The authenticated
three-file binary bundle and its restricted stager remain unchanged. Go owns its controller
implementation. Historical v0.1.6 processes do not magically participate after
files change; default installation activation remains deferred.

## Qualification

Outside-in Ginkgo/Gomega RED must exercise the compiled Go CLI and actual
legacy script entrypoints against a real exclusive flock before production
changes. Test a paused native help process before admission, manual checks,
nested bounded execution, normal cleanup and abrupt controller death. Negative
controls must cover disabled/read-only calls, unsafe storage and lock identity
replacement. Independent collaborator specs exercise retained uncertainty and
durability faults. Real Go/Python participants must agree on the inode and
shared/exclusive semantics on Linux and macOS. Existing budgets and loop suites
and source-installation/upgrade asset checks remain required.

Source progress stays within the existing fixed conversion plan. This component
does not earn controlled restoration, installed activation, script retirement,
retention, release qualification or default cutover credit. Do not claim an
installation quiescent or a v0.1.6 upgrade seamless from these tests.

## Consequences

Normal completed work leaves only two bounded control objects: the permanent
lock and an empty activity directory. Failed or interrupted work may leave
markers for inspection; no age-based pruning silently discards ownership
uncertainty. A later explicit recovery contract must reconcile these markers
with process and accounting evidence before removal. Original budgets and
checkpoint history are never rewritten by the guard.
