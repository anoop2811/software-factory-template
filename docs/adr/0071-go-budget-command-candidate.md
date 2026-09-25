# ADR-0071: Go budget command candidate

Status: accepted for source implementation under specs/001-go-runtime-conversion.md.

## Scope

Decision 70 advances the interface portion of the existing budget-controller
package. Add `FACTORY_BRIDGE_PROTOCOL=1 factory budget controller
<plan|run|report> ...` as a private candidate boundary. The installed public
`factory budget` continues its existing dispatch; no installer changes, backup,
retirement, new dependencies, network service or paid validation are introduced.
Reuse the current Cobra dependency, budget Configuration/MakePlan/Ledger/Runner,
and native/usage components. Do not copy budget policy or process supervision.

The immutable oracle is 4cc771e894e11d5024032106aeb0ef788997bd29,
scripts/lib/budget.py:289 (rendering), :472 (run ordering) and :593 (arguments).
This links source work to AC1.1/1.3, AC2.1/2.4 and AC6.1/6.4/6.5 without claiming
those cross-cutting acceptance criteria or the budget package fully closed.

## Command contract

Use Cobra in an internal command adapter with explicit stdout/stderr and context.
The bridge forwards command operands literally and returns the adapter status;
it must not add a second diagnostic or reinterpret the adapter's flags.
Support exact plan/run/report tokens; --json, --session, --task, --harness,
--role, --max-cost-usd and --prompt-file in their existing command scopes,
including --flag=value and interspersed operands. Default role is implementer.
Reject missing required fields, invalid IDs/harness/role, unknown flags, extra
positional operands and help/completion requests without invoking a harness or
creating state. Empty explicitly provided IDs fail validation. Duplicate scalar
flags retain the last value, matching the baseline.

This private candidate deliberately refuses help/completion and abbreviated
long flags. Python argparse help/error prose, abbreviation/short-option edge
cases and exact argument diagnostic ordering remain explicit compatibility work
before public activation; do not silently represent Cobra defaults as parity.
Errors use a single `factory budget: ` stderr diagnostic and code2 before
admission. An execution result retains its controller exit code (0/1/2/124/130).

FACTORY_BUDGET_ROOT selects the ledger/native checkout; unset means caller cwd.
Prompt file paths remain relative to caller cwd. Capture environment once for
configuration, selected FACTORY_BUDGET_MODEL and native preparation. No YAML or
shell-role model selection is reimplemented: this boundary consumes the exported
settings that future shell adapters will supply. Report reads/validates history
before unrelated execution configuration, so malformed execution settings cannot
hide recovery/accounting. Plan/report remain read-only and never probe CLIs.

## Rendering and ordering

Add typed shared renderers in internal/budget for Plan, ReportData and Record.
Use existing typed fields and private record data directly, never marshal then
parse to recover values. JSON emits one newline-terminated object per event;
run emits the locked plan and final record, never RunResult or raw answers.
Differential JSON comparisons normalize object-key order, whitespace, Unicode
escape spelling and numerically equivalent JSON number spellings only. They
must retain null vs absent, field types, values, arrays and event order.

Text plan/report/record uses the baseline labels, line order, fixed three/six
fractional places and unknown-cost wording. Preserve large integer counters and
Python-style scalar numeric display where baseline text exposes it. No prompt,
raw native output or full provider payload appears in metadata or diagnostics.
Only successful text runs may print an explicitly selected answer, after final
metadata publication and output. JSON runs must never select or print answers.

Extend Runner input with an optional fallible plan observer. Initial blocked
plans call it before returning without preflight or writes. For initially allowed
runs, invoke it on the current locked/recomputed plan before reservation write,
including a locked blocked plan. Emit exactly once; preflight or prompt failure
emits no plan. Observer failure prevents reservation and launch. Ledger.Admit's
existing standalone contract remains unchanged, sharing the internal admission
path with a nil observer. No callback retry or unlocked admission decision.

Writers propagate errors. A partial/unwritable plan must prevent launch; final
record or answer output errors return nonzero without retrying execution or
rolling back durable accounting. Renderers must not silently accept short writes.
Flush a writer exposing Flush() error after each logical event; ordinary file
writers are already immediate. No in-memory buffering across native execution.

## Independent acceptance

Before implementation, independently authored Ginkgo/Gomega cases must fail
through the compiled factory bridge for the intended missing command behavior.
Use immutable baseline plan/report fixtures for text and JSON comparisons,
including unknown costs, session filters, huge counters and report with invalid
execution configuration. Exercise disabled no-effects and invalid command input.
Fake CLIs for Codex, Claude and OpenCode prove live plan output before stdin,
exact JSON event count/privacy, text answer after final metadata, failed native
status and preflight refusal. Internal collaborator tests cover observer and
writer errors before admission plus durable-state preservation after output
failure. No real provider calls. Preserve immutable nondeterministic-field
normalization lists in test evidence and do not mark parser gaps complete.

## References

Existing Cobra command APIs are used by internal/cli/cli.go and
internal/bridge/bridge.go; verify the pinned module source before extending them.
Go JSON encoder documentation fetched 2026-09-23:
https://pkg.go.dev/encoding/json#Encoder.SetEscapeHTML. No dependency pins change;
future additions remain subject to the permissive-license rule in ADR-0070.


## Locked observation after preflight

Runner already completes the initial read-only check. Its controller-specific
admission path must then reach the lock and recheck the current plan, rather
than returning from a second unlocked blocked check. This ensures a competitor
consuming the last slot during preflight produces exactly one current blocked
plan without reservation or launch. Standalone Ledger.Admit retains its own
initial read-only check. Share validation and locked admission implementation;
do not duplicate reservation construction or change public standalone behavior.

Pinned Cobra source also fetched 2026-09-23:
https://raw.githubusercontent.com/spf13/cobra/v1.10.2/command.go confirms literal
flag forwarding, explicit output writers and context execution; no version change.

The plan observer receives detached typed values, including mutable integers,
maps and slices. Presentation must not mutate the ledger's policy or reservation
through aliases; copy without a JSON round trip.

Boolean switches preserve argparse's store_true grammar: --json takes no value;
--json=true/false and help flags (including --help=false) are refused. Equals
forms in the candidate contract apply to scalar string options.


## Final output after cancellation

When Runner returns durable final metadata, render that metadata and any eligible
answer with one fresh five-second context detached from parent cancellation.
This preserves interrupted/timeout controller status and final JSON event order
even when the parent is already canceled. Admission-plan output still uses the
parent context and cannot authorize a launch after cancellation. Actual writer
or flush errors remain nonzero and do not retry or rewind the run. The output
context bounds cooperative presentation work; it cannot interrupt an arbitrary
blocking writer syscall, as with the existing local filesystem qualification.
