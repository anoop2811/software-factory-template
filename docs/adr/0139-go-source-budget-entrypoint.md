# ADR 0139: native source budget command

Status: accepted implementation boundary; qualification and merge pending.
Decision: 154. Implements R3 group 5 after merged PR #169.

## Context

The public Go `budget` command already owns configured preparation, model
selection, admission, accounting and native process supervision. The direct
source `scripts/factory-budget.sh` still prepares shell configuration and calls
the Python controller. Remove that duplicate source dispatch without introducing
a second Go implementation or changing installed generation recipes.

## Source transport

Replace the direct script with an executable adapter to its physical checkout's
`factory budget`. Use the native Bash source-array identity guard already
qualified for source check orchestration. Preserve caller cwd, literal arguments,
environment, stdin, stdout, stderr and command status. Support directory aliases,
spaces and newline paths, and Bash's readable PATH-selected script invocation.
Refuse inherited scalar source metadata and a final-file symlink before handoff.

The source launcher owns any required development compilation. Missing or failing
owners and compilation failures must not select another checkout, PATH factory,
saved script or Python controller. Do not source configuration/model readers or
invoke Python to locate the owner. Forward all operands to the existing command;
do not rescan role/harness arguments or alter caller budget settings in shell.

## Native budget behavior

Reuse `configuredcmd`, `commandenv` and the native budget controller unchanged
unless an independently reproduced defect requires a separately recorded bounded
correction. Configuration preparation still precedes argument parsing. Preserve
disabled/read-only behavior, explicit configuration/caller project selection,
all three harnesses and role-tier model selection, admission limits, private
metadata and status/diagnostic behavior. Real execution evidence uses local fake
harnesses only; no paid model requests or live authentication.

Prove a budgeted native invocation through the direct adapter, including charged
failure/no automatic retry and cancellation with owned-child cleanup. Existing
native command/controller/ledger tests remain independent evidence and must not
be weakened or repurposed as adapter substitutes.

## Historical regression custody

Retain the 32 historical budget registrations rather than claim every compound
fault matrix has been reimplemented by this source adapter. Pin pre-cutover
runtime inputs to the existing immutable fixture revision
`1754de23de95a7e3c734bedd86894295e823c988`, with complete dependency inventory
and explicit development-only invocation. Git object comparison confirmed both
budget/loop wrappers and all four Python modules are byte-identical there, at the
proof inventory revision `f3a0b6520917e104883cc89b123d7244fa0c46a7`, and at merged
pre-cutover source `b9dfdba5979ba120843eb10689e90debe6245dc4`.
Preserve existing dispatcher, reader and other command oracle identities where
already qualified.

Each affected fixture must keep its intended legacy controller when the current
budget path becomes an adapter. This includes budget/loop Python fixture builders
and Go transition fixtures that deliberately exercise mixed legacy/native calls.
Keep all assertions and fault injection; freeze implementation inputs, not current
policy subjects. Record B01-B32 retention and run the affected suites before
closing their custody disposition. No historical inputs may become a production
fallback or enter installed self-check discovery.

The four active Python modules remain until group 6 closes the loop boundary and
their other consumers. This group adds no deletion authority over adopter files,
changes no installed recipes, and does not close R4 integrated release rehearsal.

## Independent qualification

The spec-writer owns Ginkgo/Gomega acceptance and observes source-boundary RED
before the implementer changes production. Reuse qualified fixture builders and
native budget expectations. Cover physical owner/transport, missing owner and
compiler refusal, no-Python configured behavior with real native execution,
literal model/role operands for Codex/Claude/OpenCode, failure accounting and
cancellation. Discover the actual source partition census before updating counts.

Run actual source compilation, enforcement hooks and all qualification only in
private complete checkouts; preserve the primary ignored operator binary and
unrelated user files. Record focused native/source and affected historical runs,
full source gate, independent verified review and hosted checks separately.
Do not relax deadlines or assertions merely to obtain a pass.

## Progress

PR #169 merged as `b9dfdba5979ba120843eb10689e90debe6245dc4` on 2026-11-06 UTC.
R3 now has 25/32 paths merged (78.125%). This one-path group reaches 26/32
(81.25%) only after qualification and merge, leaving loop/Python retirement
and public selftest as the final two R3 groups. R4-R7 remain separate release work.

## Hangup cancellation correction

Before implementation, independent real-process probes found that the legacy
budget controller catches SIGHUP, while native configured commands catch only
SIGINT/SIGTERM. After an admitted native child's stdin/PID readiness, SIGHUP
terminated the Go parent while leaving the child alive and its ledger row active.
The SIGTERM control instead exited 130, reaped the child and recorded interruption.

Preserve the budget command's legacy hangup contract through the shared configured
command cancellation mechanism. Add SIGHUP to the existing notification context;
reuse current ownership cleanup/accounting rather than a second signal handler.
No paid retry or silent reservation release may follow cancellation. Observe an
independent regression fail before this change, then require owned-child exit,
completed interrupted accounting and caller-group survival after readiness.
Retain existing SIGINT/SIGTERM checks and qualify the shared command boundary.

The legacy loop also calls the budget execution helpers for model runs and
deterministic checks, inheriting their SIGHUP handling. Keep the notification
change in the shared configured-command owner. Add one loop hangup case to the
existing public cancellation matrix, retaining its observed-child, single-call,
ledger and no-fallback assertions and checking the interrupted loop checkpoint.
This qualifies the shared correction without changing the direct loop adapter.

## Historical dependency inventory

The budget closure is its wrapper plus `budget.py`, `budget_adapters.py` and
`runtime_transition.py`. Pin it in budget/loop subset and full delivery fixtures,
the general/Java historical templates and the two Go transition fixture builders.
Python fixture materializers preserve module mode 0644; Go private fixtures keep
their existing restrictive modes. The loop wrapper and `loop.py` remain current
subjects until group 6; current policy-gate inputs remain separate from oracles.
