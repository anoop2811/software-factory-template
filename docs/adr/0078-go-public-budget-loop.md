# ADR-0078: Public budget and loop commands in the source-built Go runtime

Status: accepted for source qualification; installed activation pending.
Decision: 77. Date: 2026-09-26 UTC.

## Scope and measurement

The developer-built cmd/factory binary runs public `factory budget ARGS` and
`factory loop ARGS` through the qualified Go command environment and controllers.
Selecting that local source binary is the opt-in: add no new rollout environment
switch, configuration key, command alias or runtime-selection policy. The tracked
shell factory, direct scripts, native adapters, installer and upgrader remain
unchanged. No official artifact activation, replacement or retirement follows.

This qualifies milestone two of the existing G2 integration/retirement package
in ADR-0077. Keep the same 30-package denominator and three equal milestones.
Qualification does not close mixed-runtime activation/retirement or G4 delivery.
The immutable shell wrapper oracle remains
c8f8d34edbc14df5655fcbf0aab7eea46ced0295. Use the immutable dispatcher already
recorded by the foundational acceptance suite for unchanged script commands.

## Public routing contract

Preserve the existing first-token dispatch contract: no command, empty first
command, help, -h and --help produce top-level help and ignore remaining operands.
Unknown commands retain their existing refusal channel and status. Private
FACTORY_BRIDGE_PROTOCOL selection retains precedence over public dispatch.

For exact budget and loop command names, forward every remaining argument intact
to the same configured-command runner used by the private configured routes.
Preparation precedes leaf argument parsing, including help. Do not introduce a
second parser or reinterpret leaf flags in the root Cobra command. Preserve
adapter statuses, stdout/stderr separation and inherited input. In particular,
nonzero command results must not become successful Cobra execution, and controller
diagnostics must not be printed twice by the outer dispatcher.

The source binary must execute these two commands without colocated scripts.
A present script, including a customized one, is never an implicit fallback after
success, refusal, preparation failure, cancellation or a model/check failure.
This source binary explicitly selects Go behavior; existing direct script calls
remain available unchanged until a separately approved migration.

The other nine public script routes retain their exact exec boundary, including
script discovery, literal arguments, cwd, environment, inherited streams, process
identity, signal behavior and existing missing/nonexecutable-script diagnostics.
No changes to their implementation or registration are authorized by this slice.

Correct the compiled help footer to exactly:
`Budget and loop use the Go runtime. Other commands use auditable scripts.`
Keep the rest of top-level help bytes and alias behavior unchanged. This single
source-candidate prose difference is explicit; do not broadly normalize output.
The installed shell help text remains unchanged.

## Shared orchestration and failure boundaries

Extract one small concrete configured-command runner used by both public and
private boundaries. Reuse commandenv, budgetcmd and loopcmd; do not copy parsers,
model routing, budget policy or loop lifecycle logic. Share environment capture
without global mutation. Scope SIGINT/SIGTERM cancellation to native command
execution and restore handlers on return. Ordinary delegated script commands
retain their existing exec signal behavior.

Preparation errors are sanitized, reported once with the boundary's own prefix
(factory for public, factory bridge for private), and return 2 before accounting,
checkpoint writes, checks or model probes. Existing command diagnostics/statuses
remain owned by the corresponding adapter. No shell fallback, automatic model
retry or second paid invocation may follow an error. Preserve cancellation and
process ownership behavior of the existing controllers; this slice adds no new
cleanup/deadline promises for unsupported descriptors or regular-file syscalls.

Carry forward the enumerated ADR-0077 qualifications: stable configuration/cwd,
bounded Git discovery, sanitized configuration I/O failure, and explicit PATH
presence (empty allowed; absent refused). The absent-PATH mismatch with Bash's
build-dependent default remains unresolved and blocks installed activation for
that input. Public source qualification is not a claim of unconditional shell
parity, nor permission to activate an adopter runtime with open compatibility gaps.

## Independent outside-in qualification

The evaluator owns Ginkgo/Gomega tests and observes RED before implementation.
Run actual public compiled commands with private protocol unset and no scripts,
plus poison-script sentinels that fail if budget/loop delegation occurs. Compare
plan/report/status/manual execution and bounded fake-client execution against the
immutable wrappers and previously qualified command semantics. Exercise all three
harnesses, model/role choices, empty overrides, enabled/disabled budgets and loops,
argument/help failures, command status propagation and no-effects refusals.

Prove unknown commands do not prepare configuration, native command failures do
not execute a fallback script, and private configured routes retain their existing
behavior. Exercise real subprocess cancellation with a write/readiness handshake,
never a sleep used as readiness; confirm no unintended retries or leaked children.
Keep all nine unaffected script-route tests, including PID/stdin/path/signal cases.
Update only the two retired source-binary delegation expectations and the exact
help footer assertion; do not weaken the rest of the legacy boundary matrix.

Run focused race-instrumented CLI acceptance, existing private command regressions,
and the complete Go source gate. No paid model or live native-client calls during
qualification. No dependency/toolchain pins or additional libraries are required.
