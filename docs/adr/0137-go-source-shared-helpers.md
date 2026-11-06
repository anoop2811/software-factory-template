# ADR 0137: native source events, timing, color and hooksPath helpers

Status: accepted design; implementation and qualification evidence are separate

Decision: 152

## Scope and authority

Complete R3 group 3: `scripts/lib/events.sh`, `scripts/lib/timing.sh`,
`scripts/lib/color.sh` and `scripts/lib/hookspath.sh`. The existing private Go
helper protocol owns event append/retention, time formatting, palette/box output
and Git hooksPath classification. Reuse those implementations and the canonical
shell helper plans; do not introduce another parser, formatter or classifier.
Installed generated adapters and immutable installation recipes retain their
existing bytes and ownership rules.

## Bash source ownership

Bash-capable callers load canonical `runtime/shell/helpers.sh` definitions and
`runtime/shell/source.sh` by absolute physical paths derived from the public
library. Later caller directory changes and ambient runtime/source-root values
must not select a different Bash owner. Preserve literal directory aliases and
newline paths; reject final-file aliases where Bash supplies their identity.
Use actual Bash capability rather than inherited metadata alone. Resolve paths
with builtins so ordinary caller cd/pwd/printf wrappers add no source-time effects.

Events, timing and hooksPath sourcing must not invoke Git, Go or a native owner,
or change caller cwd, options, IFS, positional arguments or controls. Color
intentionally applies its canonical palette plan at source time. Caller-local
and readonly control values reach only child environments. Canonical plan
validation precedes every palette assignment; action boxes receive literal
caller palette overrides, titles and lines.

## POSIX source callers

A POSIX shell cannot generally locate a sourced file. Preserve inert loading for
non-color helpers through thin literal forwarding functions: at invocation,
select an explicit absolute regular executable `FACTORY_RUNTIME_BINARY`, else
the caller Git repository's executable `factory`. A supplied invalid or failed
runtime never falls through to another owner. Outside Git, these helpers need
the explicit runtime. Shared computation remains in Go; minimal standalone
locator/forwarding glue is justified where inert sourcing cannot load a sibling
file whose path the shell cannot discover.

Color must load the canonical palette-plan interpreter in the caller shell.
For a true POSIX source caller, locate that interpreter through an explicit
absolute `FACTORY_SOURCE_ROOT` source checkout, otherwise the caller Git root.
Outside Git, source color therefore requires that explicit source root; an
arbitrary standalone runtime alone cannot identify the canonical shell source.
A supplied invalid root fails visibly without an alternate lookup. This is a
source-only location contract: ordinary Bash callers and generated installed
adapters require no new setting. Runtime selection remains explicit runtime,
then explicit source root for color, then caller Git root; never assign caller
runtime/configuration/protocol variables merely to select the child.

Missing native ownership preserves existing helper-specific failure behavior:
event logging is quiet best effort and returns zero; now returns zero text;
duration returns unknown; hook classification and action-box/palette failures
remain nonzero. None may fall back to a saved Bash implementation. Actual TTY
and nonempty NO_COLOR determine palette values through the native owner.
For events, an unavailable canonical bridge at source time also leaves a quiet
no-op logger and returns success: sourcing telemetry under `set -e` cannot change
a gate decision. This is failure containment, not a legacy implementation fallback.

## Installer bootstrap exception

The POSIX installer measures a cached-source refresh before native handoff is
authorized. It must not source the fetched timing library or execute its runtime
merely to print elapsed time. Retain its existing bounded local date/arithmetic
fallback as bootstrap behavior; remove the optional downloaded-library sourcing.
Bare fetch and refresh remain Git/bootstrap operations, without compilation or
execution of selected source code for timing. This exception does not put a
second helper implementation behind any public factory runtime command.

## Current consumers and historical custody

Current policy fixtures must include all current helper adapters, canonical
bridges and a private native owner. Source initialization and source selftest
must exercise these actual helpers. Structural evaluation receipts distinguish
helper events from exact evaluation/configuration calls; retain every assertion
and require the actual denial event rather than accepting arbitrary extra calls.
Logging failure must not alter a real gate's deny/allow decision.

Historical init/upgrade/doctor/metrics consumers receive coherent helper bytes
from their existing immutable fixture revisions. Do not freeze live event,
retention, timing or hooksPath assertions globally. The general metrics fixture
must retain its real current event checks with a private native owner, even
where the metrics controller itself is historical. Preserve current optional-pack
dependency guards and all existing outcome checks. Installed recipes, poison
libraries and independent native differential oracles remain unchanged.

Private compiler outputs belong under disposable qualification directories;
reading checkout source for a private `go build -o` is permitted. Never invoke
the operator's source launcher or replace its `factory`/`factory-go` as a fixture
side effect. Qualify the actual evaluator Git within restricted test PATHs.

## Qualification and progress

An independent test author first observes old helper bodies fail native-boundary
criteria. Cover Bash ownership and literal forwarding, inert POSIX loading,
explicit-owner refusal and fallbacks, actual source compilation with gate-event
behavior, pipe/TTY/NO_COLOR palette behavior, real Git hooksPath classification,
and installer fetch/refresh without selected-runtime execution. Reuse existing
native helper semantic matrices instead of duplicating them. Do not add Python
or another dependency for new tests.

Run affected historical/current checks, source race acceptance and quality gates;
verify independent review findings before responding and obtaining hosted review.
No timeout or criterion is removed to make a check pass. R3 is now 20/32 merged
paths (62.5%); this four-path group moves it to 24/32 (75%) only after qualification
and merge. The remaining check, budget, loop/Python and selftest groups and R4-R7
release gates remain separate work.
