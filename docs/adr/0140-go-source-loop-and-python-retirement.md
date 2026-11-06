# ADR 0140: native source loop and Python runtime retirement

Status: accepted implementation boundary; qualification and merge pending.
Decision: 155. Implements R3 group 6 after merged PR #170.

## Context

The Go loop command already owns manual and bounded execution, checkpoints,
resume validation, budget admission and process supervision. The remaining
source loop wrapper still dispatches to Python. Budget source routing merged
in PR #170; this group closes the last controller dependency before retiring
the four active Python runtime modules. Public selftest remains group 7.

## Source transport

Route `scripts/factory-loop.sh` to its physical checkout's `factory loop`, using
the already qualified budget adapter transport. Preserve literal argv, caller
cwd, environment, streams and status. Do not read model/configuration values in
shell, select a PATH factory, or fall back to a historical implementation.
Refuse forged scalar Bash source metadata, a symlinked adapter file and a
missing/non-executable owner. Preserve directory aliases and newline paths.
The source launcher retains sole ownership of development compilation.

Reuse the existing configured-command and loop engines. Manual execution stays
manual; bounded execution stays explicitly enabled and budgeted. Preserve
checkpoint identity, admission limits, stopped/completed outcomes, no automatic
paid retry, cancellation and owned-child cleanup. Use local fake harnesses for
Codex, Claude and OpenCode, never paid model calls. A runtime correction requires
an independently reproduced defect and a recorded decision before code.

## Runtime retirement and historical custody

Delete the active `scripts/lib/budget.py`, `budget_adapters.py`, `loop.py` and
`runtime_transition.py` after accounting for every consumer. Remove them from
the current initializer input inventory. Keep immutable installation recipes,
legacy ownership catalogs and retirement rules intact: they reconstruct or
identify predecessor installations and are not current execution fallbacks.
Repository deletion grants no new authority to remove customized adopter files;
the existing assessed ownership, recovery, rollback and retention rules apply.

Retain loop L01-L40 as historical development regressions. Pin their complete
wrapper/module dependency closure to the existing immutable fixture revision
`1754de23de95a7e3c734bedd86894295e823c988`. Preserve other deliberately frozen
oracle identities. Current policy subjects stay current. All full and subset
materializers, including transition and native-init differential fixtures, must
receive identified historical bytes before current production files disappear.
Do not weaken fault injection, replace an assertion with a skip, or infer
compound-case equivalence from an unrelated native suite's passing count.

Historical regressions may execute Python only in isolated development fixtures;
no source/installed public command may discover those fixtures as a fallback.
The public selftest separation remains its own next group. Update baseline
specification references to surviving acceptance evidence, and classify retained
historical references explicitly rather than deleting reconstruction knowledge.

## Independent qualification

The spec-writer owns outside-in Ginkgo/Gomega tests and observes RED before
production edits. Qualify the real loop adapter's owner selection/refusals and
native execution without a Python tool, including read-only behavior, manual
checkpoint/resume, bounded three-harness calls, accounting and real cancellation.
Reuse existing transport/process helpers where their authority matches this
boundary. Preserve failure-path cleanup and do not relax timing or ownership
rules to obtain a pass.

Independently qualify absence of current runtime modules and coherence of current
initialization/copy inputs while retaining old recipe reconstruction. Run affected
native loop/configuration/transition/init criteria and historical budget/loop,
general/optional/Java regressions. Compare historical registered function ASTs,
discover the real acceptance partition census, and run the enclosing source gate.
Execute compilation, enforcement and tests only in disposable complete checkouts;
preserve the operator's ignored binary and unrelated local changes. Record local,
independent review and hosted exact-head evidence separately before merge.

## Progress

PR #170 merged as `9b194645c34ed116863d8f245916e19eddfa7d0b` on 2026-11-06 UTC.
R3 now has 26/32 paths merged (81.25%). This five-path group reaches 31/32
(96.875%) only after qualification and merge. Public source selftest then remains;
R4-R7 integrated rehearsal, candidate qualification, pilot and stable activation
are still separate release obligations.

## Historical initializer component boundary

The existing test-only initializer driver exercises current `initcmd` against
an archived legacy scaffold and compares its complete historical asset set.
Its Go-transition delivery cases intentionally install the old Python exclusion
shim. Preserve this particular component contract with a build overlay of only
`internal/initinventory/inventory.go` from immutable `1754de23de95a7e3c734bedd86894295e823c988`.
All initializer engine code and assertions remain current. The historical driver
is never the shipped candidate CLI. Independently exercise current inventory
retirement through the unmodified candidate/current source copy and initialization
boundaries, so historical retention cannot hide a current packaging defect.

## Interrupted-recovery oracle custody

The interrupted-recovery acceptance probe imports `runtime_transition` by module
name through a source-tree `sys.path`, so a filename-only dependency search misses
it. Retain its actual Python shared/exclusive admission assertions by materializing
the immutable transition module in a separate private test directory. Keep that
oracle outside the adopter tree whose unchanged bytes the recovery criteria
assert. Do not restore a production module or weaken pending-barrier assertions.
Audit bare imports and dynamic module/path construction as well as literal
filenames before declaring the retirement reference inventory complete.
