# ADR 0132: native source golden-evaluation entrypoint

Status: accepted design; implementation and qualification evidence are separate

Date: 2026-11-02 UTC

## Scope

Replace the remaining direct `scripts/golden-task-eval.sh` implementation with
an adapter to the existing `factory eval golden` owner. `make golden-eval`
continues to use that public path. Reuse the established native evaluation
contract in docs/adr/0099-native-factory-orchestration.md:104; do not introduce a
second evaluator, a dependency, a model call or a release-default change.

This is one R3 function boundary. Remaining direct commands, sourceable helpers
and historical proof-runner retirement are separately accountable work. Passing
this boundary does not close R3 or the 0.2.0 release gates.

## Invocation and results

Use the established physical-owner source adapter. Refuse final-file symlinks,
resolve the owning source checkout without changing the caller's working
directory, and exec its `factory eval golden` with literal original operands.
Preserve stdin, environment, output streams and exit status. Directory aliases
and owner paths ending in newlines must retain their actual identities. Neither
PATH nor a runtime environment override may select a different owner; a missing
or failed owner fails without a legacy fallback.

The caller's project supplies tasks, runners, configuration and results. Keep
strict spaced/equal flag parsing, deterministic default mock selection, explicit
runner precedence, complete result/baseline schemas, independent checksums and
baseline staleness/regression behavior. Unknown harnesses never select a mock or
paid fallback. Native timeout, cancellation and oracle-integrity behavior from
ADR 0099 remains authoritative. Preserve baseline identities when fingerprinted
inputs match; changed runners, policy or model settings still make evidence stale.
Never rewrite a baseline identity to make earlier evidence appear current.

The converted command has no Python scorer or Python prerequisite. Source
execution retains the existing Go compiler requirement; installed execution uses
its packaged Go binary. Product-task or explicitly selected runner dependencies
remain the caller's responsibility. No paid harness is used for qualification.

## Historical regression custody

Retain every meaningful existing regression. A historical fixture that intends
to run a frozen Bash dispatcher must obtain its golden evaluator from the same
identified historical source, rather than accidentally calling the new adapter.
Historical bytes remain explicit test data and cannot be a production fallback.
Update or replace the prior direct-script Python-prerequisite regression with
independent native routing and no-Python evidence; its former failure is now the
conversion's semantic RED. Do not weaken runtime task-oracle, timeout, baseline
or runner assertions to accommodate routing changes.

## Qualification

An independent test author first observes the current direct script fail a
real successful evaluation without Python. Then require direct-path and Make
invocations to reach the native owner and publish the expected schema and score
in the caller's project. Qualify at least one real private source launch and use
the already compiled candidate for remaining focused cases. Preserve the user's
primary checkout binary and unrelated files throughout qualification.

Exercise exact transport, physical/directory aliases, file-symlink refusal,
missing/failed owners, wrong-runtime decoys, strict parsing, no-task behavior,
no-Python scoring, baseline save/comparison/staleness and oracle tampering. Reuse
existing native golden-evaluation acceptance for command semantics rather than
duplicating its full implementation in tests. Use independent expected values
and owned fake runners; never infer success merely from an adapter's text.

Run affected native and historical suites, quality gates and the review diamond.
Record actual RED/GREEN commands and results. Hosted checks and review responses
must precede any completion or merge claim. Release qualification remains separate.

## CI qualification follow-up

PR #163 remains based on merged PR #161; the stopped scheduling and parallelism
work in PR #162 is not a dependency. Repair only the consumer-fixture preparation
that exhausted existing macOS budgets. Keep workflow topology, worker selection,
the 25-minute suite deadline, the 90-second image deadline and behavioral
assertions unchanged.

Default initialization and upgrade fixtures may invoke the real compiled
`cmd/factory-package` with the same explicit source, revision, version, target,
compiler and installation options instead of calling its library inside the
race-instrumented test process. This is setup for a consumer test, not a substitute
for the dedicated race-enabled packager tests. Compile the setup utility once
per fixture, keep compilation and its first invocation within the existing image
deadline, supervise subprocess completion, and decode its actual JSON result.
Every selected image must still come from the production packager and the
committed selected source; no fabricated manifest or reused unqualified binary.

Test-owned tar/gzip fixtures may use fastest compression while preserving every
decoded header and payload. Production archive compression remains unchanged.
This supersedes only ADR 0105's restriction that the shared test archive encoder
retain its previous compression level; its payload, boundary and timing-evidence
requirements still apply.
Compare decoded entries and run the real handoff and default-init/upgrade
consumers before claiming qualification. Existing race instrumentation and all
criterion identities remain in place. A local timing sample does not establish
a hosted pipeline improvement; hosted success must be observed on the new head.
