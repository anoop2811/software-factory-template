# ADR-0076: Go bounded loops and command integration

Status: accepted for private source qualification; installed activation pending.
Decision: 75. Date: 2026-09-25 UTC.

## Scope

Complete milestone four of the existing loop package defined in ADR-0073:
bounded shared-budget implementation/check/review/repair and command integration.
Oracle: scripts/lib/loop.py at 76952eaa63aebd1ecd282f5ab51dd7c3627cb497, with
explicit inherited qualification limits from ADR-0073/0074/0075 and the budget
controller ADR-0070. Do not activate installed dispatch or remove legacy runtime
before delivery/migration acceptance. Keep existing private loop requests intact.
Compose existing manual lifecycle, native supervisor, checkpoint storage,
fingerprints, budget controller and response parser; avoid duplicate engines.

## Command candidate

FACTORY_BRIDGE_PROTOCOL=1 factory loop command ARGS forwards literal operands to
a Cobra-based loop command candidate supporting plan, run, status and resume.
Preserve Python command semantics, exits, JSON shape and human output structure:
session/task required throughout, JSON optional, harness required on plan/run,
optional on resume (infer existing harness), unavailable on status; mode defaults
to manual, accepts manual/bounded; prompt-file optional on run/resume and required
only after admission for bounded execution. Status reads whole validated history
and returns the selected record without parsing unrelated configuration or writes.
Use FACTORY_BUDGET_ROOT or cwd for this legacy-compatible command boundary; prior
private loop root rules remain unchanged. Exported FACTORY_LOOP_* configuration
and resolved model variables are supplied by the caller; no native config rewrite.

Reuse the already-qualified argparse-compatibility grammar from budget commands
through a small shared concrete helper if needed. Keep Cobra registered metadata
as the source of option names/arity/required flags. Cover unique long prefixes,
ambiguous prefixes, duplicates/last value, equals/separate values, negative values,
help precedence, unknown operands, literal shell metacharacters and terminators.
As in ADR-0072, normalize diagnostic prose/help layout, but preserve status and
stdout/stderr destination and pre-effect refusal. Never reflect raw operands,
prompt text, model responses or arbitrary internal errors in diagnostics.
Plan and status never read prompts or probe native clients. Plans validate stable
source even when blocked, match Python blocker order and never create state.
Human output follows Python emit; JSON is one value plus newline. No partial
terminal record after publication failure. Output uses bounded cleanup as ADR-0075.

## Shared admission and lifecycle

Manual behavior stays qualified by ADR-0075. Bounded planning requires both loop
and budget enabled; combines global active-budget and active/uncertain-loop
blockers, check-command readiness and budget.MakePlan for implementer with its
selected model. Role models are FACTORY_LOOP_<HARNESS>_<ROLE>_MODEL; blank means
inherit. All invocations share the same session/task and ledger limits. Reviewer
calls consume budget attempts too; never increase configured limits silently.

Run holds the persistent loop lock throughout. Repeat whole-history admission
and fresh observations under lock; reject duplicate identity and corrupt state.
Load bounded prompt as UTF-8 with Python universal newline normalization; limit
normalized UTF-8 to 1 MiB, reject unsafe nonregular inputs without blocking, and
bound actual reads. Empty prompt-file name is missing as in Python; manual ignores
prompt-file without reading it. Preserve prompt digest and re-read/re-digest the
file on freshness checks. Preserve snapshot/policy/HEAD/safety checks around every
check and review and after implementation. Concurrent user edits are not rolled
back. Changes to tests/protected/governing inputs stop for human inspection.

Resume remains manual-only, with exact harness/mode/freshness and elapsed rules.
A terminal bounded record never automatically relaunches models. Status exposes
stopped/completed and useful next_action; no recovery/reset command is added.

## Bounded iteration

Start a schema-one bounded record as active/starting, preserving existing fields.
Create one absolute loop deadline and carry it through saves, probes, native
preflight, budget admission, execution and freshness checks. Keep prior five-second
cleanup/publication bounds and poisoned-transaction refusal. Initial publication
and lock semantics remain those of the existing controller. The stricter loop,
budget and check allowance wins; remaining time cannot grow across iterations.

Before each implementer launch: fresh observation, increment attempts exactly,
save phase implementer, then invoke budget.Runner with that role, selected model
and task prompt plus repair diagnostics. No independent native/provider execution.
Use explicit injected role; do not weaken native permission/preflight requirements.
After implementation check freshness; from the second attempt onward update
no_progress when source is unchanged, reset on changed source, and stop at its
configured limit. Compare arbitrary-size counters without narrowing overflow.

Run the existing deterministic check with merged bounded output. Extract the
shared check stage so manual and bounded paths use one lifecycle implementation.
A failed ordinary check may repair; other outcomes hand off. Track failed-state
identity as Python digest of source identity, exit code and SHA256 of full captured
raw output. A repeated identical failed state stops even if an intervening attempt
changed something else. Limit repair diagnostic output to the first 16384 raw bytes,
decode UTF-8 with replacement and frame it explicitly as untrusted diagnostic data.
Do not persist raw check output or its response in metadata.

After a passing check, invoke an independent reviewer via the same budget.Runner.
Require exact JSON object with exactly verdict/findings, no duplicate object keys,
no fences/trailing data; verdict approve requires empty findings, repair requires
1..50 nonblank strings of at most 4000 Unicode code points each. Whitespace uses
Python-compatible rules. Reject other types, unknown keys and malformed responses.
Source must equal the tested snapshot after review; deadline must remain valid
before accepting approval. Append review evidence with findings_count, not content.
Approve yields completed/approved/status0; repair feeds the original Python repair
framing plus JSON findings into the next attempt. Exhaustion yields stopped/
attempt_limit; no_progress and repeated_failure retain their named outcomes.
All other failure/handoff outcomes return2, never success based only on CLI exit.

## Budget evidence and uncertainty

Record each returned budget run identity once and append invocation evidence with
role, harness, outcome and elapsed time. Preserve ledger accounting even if a loop
save fails; never erase history or roll back paid attempts. Quiet orchestration
must not emit nested budget plans/results into loop JSON. Extend budget with narrow
read-only typed observation helpers if needed; never expose mutable ledger maps.
On error, reconcile readable matching active budget rows and typed native ownership
errors. Retain associated run IDs/PIDs and uncertainty if exit is unconfirmed;
unreadable ledger implies uncertainty. A preflight refusal with readable accounting
and no owned child is a certain handoff, not permission for retry. Budget errors,
malformed review, output limits, timeouts and signals never trigger automatic paid
retry. Only an ordinary failed check or valid repair verdict may advance iteration.
The inherited signal-ownership qualification distinction remains explicit.

## Qualification

Independent outside-in Ginkgo/Gomega RED precedes production. Use immutable Python
oracle, real compiled command and fake native clients for all three harnesses.
Observe successful approve, failed-check repair, reviewer repair, attempts, shared
budget exhaustion including reviewer calls, no progress, repeated failure, strict
review parsing, prompt/safety/source mutations, bounded-resume refusal, signals,
timeouts and publication/ownership failures. Exercise command grammar and readonly
status/plan separately. Assert no raw prompts/results/secrets in durable metadata,
no hidden launches/fallbacks and matching native role/model selections. Existing
manual, checkpoint, budget and native regressions must remain green. No real model
calls or live-client enforcement claim. Linux static checks are not runtime proof.

No dependency version change is selected. Cobra and JSON token documentation read
2026-09-25 UTC: https://cobra.dev/docs/how-to-guides/working-with-flags/ and
https://pkg.go.dev/encoding/json#Decoder.Token. Preserve exact Python value semantics
using the established JSON helpers rather than ordinary float-decoding maps.

## Prompt admission ordering clarification

The immutable oracle validates an admitted bounded prompt before creating loop
storage or acquiring its persistent lock. Preserve that pre-effect refusal for
missing or invalid prompts, then revalidate freshness under the lock before any
native invocation. The evaluator observed the candidate creating `.factory` on
missing-prompt refusal on 2026-09-25 UTC; this clarification records the inherited
ordering requirement before correction.

The command grammar inherits ADR-0072's explicitly enumerated interpreter
qualification for malformed mixed short-help groups, ambiguity after help and
root terminator before command/help. Keep the fixed Go refusal and no-effects
assertions for loop commands too; no broader status normalization is permitted.

## Prompt-read cancellation identity

Preserve context cancellation and deadline errors when the bounded prompt reader
observes them during reading, as already required before and after reading.
Do not replace those errors with a generic prompt-validation error. Return no
partial prompt. Ordinary file, encoding and size failures remain sanitized.
This clarifies the helper's error contract without changing retry permission,
caller handoff text or the regular-file syscall limitation.
