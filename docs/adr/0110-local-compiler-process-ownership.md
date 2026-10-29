# ADR 0110: supervise local compiler processes through the native executor

Status: implemented with local qualification; hosted qualification and review pending

Date: 2026-10-29 UTC

## Context

The public `init --local --source` path now uses the canonical developer packager.
Its compiler version, module-inspection and build calls still use direct
`exec.CommandContext` capture. Killing only the compiler does not close a capture
pipe inherited by a descendant. A real private rehearsal observed the compiler
die after parent-only SIGTERM while the public factory and its capture remained
blocked. Releasing the positively identified evaluator-owned descendant then
allowed exit 1. This is a local developer build, not installed Go execution.

## Contract

Use the existing native supervised process-group executor for all three compiler
operations. Preserve resolved literal compiler identity, argv, committed source,
filtered Go environment, offline policy and working directory. Preserve stdout
separately for module JSON; version/build may merge stderr as before. Bound
version output at 64 KiB, module JSON at 1 MiB and build diagnostics at the native
16 MiB ceiling. Stderr capture is independently bounded. Overflow is a failure,
never a usable truncated version/module response or successful build.

Use the caller's context and the native executor's existing bounded cleanup.
Accept results only after successful process exit and confirmed group ownership.
Retain context cancellation and typed ownership uncertainty through wrapping;
never publish a bundle on cancellation, output overflow or uncertain cleanup.
If compiler ownership remains uncertain, retain its private workspace with an
actionable path diagnostic instead of deleting bytes a descendant may still use.
Normal confirmed cleanup removes owned temporary workspaces. Preserve user source
and never delete an existing output directory.

The standalone Cobra `factory-package` command must use `signal.NotifyContext`
for INT/TERM and execute with that context, so compiler group isolation does not
create a new orphan boundary. Public local init already supplies a signal-aware
context. Preserve ordinary success, malformed-request and operational-failure
exit behavior; no new public command or cancellation-success claim is introduced.

## Independent qualification

An independent test author must observe meaningful pre-fix failure through the
real public local CLI after a compiler/descendant readiness handshake. Cover
version, module inspection and build, parent-only INT/TERM, descendants retaining
stdout/stderr and a healthy control using the same compiler wrapper. The tests
must own their processes and cleanup and must not rely on a sleep as readiness.
Observe bounded return, preserved cancellation, no bundle/target publication,
unchanged committed source, and actual descendant absence or explicit typed
ownership uncertainty. A surviving process never counts as confirmed cleanup.

Exercise the standalone packager's signal path and package-level context
cancellation with Ginkgo/Gomega. Preserve existing installation Git ownership
criteria and use the native executor's established fault-injection controls for
otherwise unobservable supervision failures. No test-only production flags,
new interpreter, external process library or paid model request is required.

This repair remains part of PR #141's local-source release preparation. Hosted
Linux/macOS checks and exact-head review must pass before merge; these written
requirements do not qualify RC4 publication or stable activation.

## Local qualification

The independent compiler/descendant handshake criterion first failed on the
previous implementation: after context cancellation, packaging still waited
beyond six seconds for its descendant's capture pipe. The fixture released only
its positively identified processes before reporting the failure.

The final outside-in command used the race-instrumented CLI and evaluator:

```sh
FACTORY_CLI_TEST_RACE=1 go test -race -count=1 -v ./acceptance \
  -ginkgo.no-color -ginkgo.focus='Local source compiler process ownership' \
  -ginkgo.fail-on-empty
```

Observed: `Ran 12 of 3093 Specs in 13.111 seconds`,
`12 Passed | 0 Failed | 0 Pending | 3081 Skipped`.
The selection includes a healthy real build, three compiler-stage cancellations,
four independent output bounds and public/standalone parent-only INT/TERM cases.
The same criteria with only the production files replaced by the immutable
previous implementation observed `1 Passed | 11 Failed` in 49.959 seconds;
the healthy control still passed and no criterion failed during compilation.

A separate evaluator-only cleanup-report fault kept the real OS kill but
returned EACCES to supervision. Its additional typed-uncertainty assertion passed
and observed the private workspace and actionable diagnostic retained. This
qualifies error propagation and retention separately from ordinary process exit.
The existing packaging race suite returned `ok` in 9.850 seconds. These local
Darwin observations do not claim a new-head Linux or published-release pass.

The complete-source public initializer was also exercised after this correction:

```sh
FACTORY_CLI_TEST_RACE=1 go test -race -count=1 -timeout=4m -v ./acceptance \
  -ginkgo.no-color \
  -ginkgo.focus='Go local source cutover packages committed local source.*explicit source directory' \
  -ginkgo.fail-on-empty
```

Observed: `1 Passed | 0 Failed` in 106.807 seconds, including real committed-source
packaging, installed initialization and installed self-tests with the compiler
poisoned afterward. The real census assertion passed with 3,093 total criteria:
2,946 base, 103 installation and 44 handoff. Independent correctness, security
and coverage reviews found no remaining actionable finding in this correction.
`golangci-lint run --config packs/go/.golangci.yml ./...` returned `0 issues.`;
`go vet ./...` and `gosec -quiet ./internal/packaging ./cmd/factory-package`
both exited zero. Hosted qualification remains pending.
