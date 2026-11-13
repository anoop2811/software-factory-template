# ADR 0150: bound read-only re-observation of an uncertain compiler group

Status: accepted and implemented; local qualification recorded below.
Decision: 166.

## Evidence and scope

PR #180 qualification at `2eb4feb` reported one macOS source-entrypoint failure
in run `43320430957`, job `131539371774`: the directory-output race returned
125 and retained private build, startup and admission files. The report states
compiler ownership was unconfirmed. This is intentional refusal evidence, not
proof that ordinary publication cleanup leaked files.

The approximately 334 ms between the preceding refusal receipt and this receipt
is inconsistent with exhausting the complete existing reap or TERM/KILL polling
windows. Initial observation uncertainty is the strongest source-backed lead;
the hosted probe's raw diagnostic and errno were not recorded.

A diagnostic-only private launcher replay retained the production decisions and
added external observation receipts. All fifty trials returned the intended
directory-publication refusal, confirmed compiler PID/group absence and removed
private temporary files. The hosted failure was not reproduced. Do not describe
an unidentified native errno or a production post-completion hang as established.

## Decision

When the initial compiler-group probe is unknown, perform at most five additional
read-only observations, separated by 100 ms. This is an explicit additional
bounded observation window, not an increase to command, suite or CI job limits.
The normal initial-present and initial-absent paths remain unchanged.

Only confirmed group absence followed by the existing terminal-child reaping
proof permits reconciliation. Use `source_reap_child`; do not enter
`source_finish_absent_group`, which can signal a live direct child. Do not signal
a group or child from this initially uncertain path.
Persist this uncertainty across signal-trap reentry: cancellation during the
observation window must refuse rather than restart the normal signaling path.
Clear the state only after confirmed absence and terminal-child reaping.

If any later observation reports a present group, if uncertainty persists, or if
the child cannot be proved terminal and reaped, retain the private artifacts and
return the existing ownership-refusal status 125. An empty or unexpected probe
diagnostic is never evidence of absence. Successful reconciliation must still
pass the unchanged output-publication guards; a competing directory or symlink
remains preserved and refused.

## Independent qualification

Add deterministic private-launcher controls that retain actual compiler launch,
completion and process identities while injecting only the bounded observation
uncertainty. Observe RED against the original initial-unknown refusal before
implementation, then qualify:

- One initial unknown followed by real confirmed absence and terminal-child
  evidence permits ordinary cleanup and reaches the intended publication guard.
- Persistent unknown and unknown-then-present observations retain evidence and
  return 125 without signaling or publishing.
- An absent group with a live, stopped or unadmitted child does not authorize
  signaling or an unbounded wait; existing refusal controls remain effective.
- The genuine directory/symlink race occupants, caller state and unrelated
  processes remain unchanged, and normal initial-present/absent behavior remains
  covered.

Keep fixture observations outside the source snapshot being checked. Preserve
every existing acceptance criterion and qualify the full source-entrypoint group,
including signal ownership, with the existing resource and time limits. A passing
deterministic regression proves the revised uncertainty policy; it does not
retroactively identify the unrecorded hosted errno.

## Local qualification evidence

Qualification used Go 1.27.4, `GOTOOLCHAIN=local`, `GOENV=off`,
`GOMAXPROCS=3` and command-scoped `caffeinate -i` on macOS ARM64.
The original initial-unknown behavior failed the independent native-entrypoint
criterion in 14.245 seconds. The additional cancellation handshake then exposed
four signal attempts from reentered cleanup before the persistent latch existed;
that independent RED took 21.217 seconds. Neither result identifies the hosted
probe's unrecorded native errno.

The focused command was:

```text
go test -race -count=1 -v -timeout=10m ./acceptance -ginkgo.focus='enters the existing native command surface from a private source checkout without changing its caller|refuses an absent compiler group without waiting on a live unadmitted child or killing its borrowed group' -ginkgo.fail-on-empty -ginkgo.no-color -ginkgo.json-report=../.factory/ci-mode-evidence/source-group-reobservation/final-green.json
```

It reported `3 Passed | 0 Failed | 3456 Skipped` in 27.195 seconds. The
receipts include seven terminal-child cases and actual live/stopped unadmitted
children. They require real process observations, no signal attempts, preserved
caller/output occupants and exact refusal/publication behavior.

The full `make go-runtime-source-check GO_RUNTIME_TEST_GROUP=source-entrypoints
GO_RUNTIME_TEST_TIMEOUT=25m GO_RUNTIME_TEST_PROCS=auto` run selected all 115
source criteria and reported `114 Passed | 1 Failed | 3344 Skipped` in 518.982
seconds. The remaining failure was fixture instrumentation: the existing delayed
cleanup helper scanned the new read-only sleep as well as the two original signal
polling sites. Scope its signal-cleanup instrumentation to the actual TERM/KILL
loop while preserving both exact site assertions. Reaper instrumentation remains
unchanged. Rerun both helper consumers before final hosted qualification; this
failed full run is not a passing receipt.

After the fixture scope correction, both consumers were rerun with:

```text
go test -race -count=1 -v -timeout=10m ./acceptance -ginkgo.focus='settles a cold source compiler group before the outer golden timeout returns|refuses an absent compiler group without waiting on a live unadmitted child or killing its borrowed group' -ginkgo.fail-on-empty -ginkgo.no-color -ginkgo.json-report=../.factory/ci-mode-evidence/source-group-reobservation/cleanup-helper-green.json
```

The result was `3 Passed | 0 Failed | 3456 Skipped` in 21.218 seconds.
The actual delayed-poll receipts and the exact two-site/one-site assertions remain
required. The full group and this targeted correction together exercised every
source criterion; final-head hosted qualification remains separate.

The lint-required switch cleanup in the terminal observation assertions was
followed by the same native-entrypoint focus alone, with the report written to
`source-group-reobservation/switch-green.json`. It reported
`1 Passed | 0 Failed | 3458 Skipped` in 20.826 seconds. Final standalone
`golangci-lint run --config packs/go/.golangci.yml ./...` reported `0 issues.`;
`gosec ./...` reported `Issues : 0`; and `govulncheck ./...` reported
`Your code is affected by 0 vulnerabilities.` The scanner also reported ten
module-only advisories, with none in imported packages or called symbols.
