# ADR 0113: isolate ordered local-cutover qualification

Status: locally qualified; fresh hosted timing pending

Date: 2026-10-29 UTC

## Observed failure

The macOS base job for PR #141 at `0cb6613` reached its twenty-five-minute suite
deadline after 2,866 passing criteria. The two interrupted configuration cases
had run for 0.608 and 0.513 seconds; neither reached its own ten-second command
deadline. The report records a suite timeout, not a failed behavior assertion.

All seven source-launcher custody criteria and all nine inventory criteria
passed. Eight ordered `Go local source cutover` scenarios also passed, consuming
550.911 seconds together on one worker. These actual packaging, initialization,
upgrade and recovery scenarios share their ordered installation state and must
remain together. Adding them to the existing base partition exhausted its
allowance; increasing a deadline would not address that scheduling pressure.

Provenance: observed 2026-10-29 via
[macOS base job 123710479452](https://github.com/anoop2811/software-factory-template/actions/runs/40948478308/job/123710479452)
and its retained `go-runtime-base-macOS-ARM64/base.json` report.

## Partition contract

Add the repository-only `source-cutover` qualification group. Its anchored
selector is `^Go factory command acceptance Go local source cutover( |$)`:
Ginkgo includes the suite prefix. Similarly named neighboring families remain in base.
Exclude that same family from base and preserve the existing installation and
release-handoff selections unchanged. The four selections must be disjoint and
their union must equal every discovered acceptance criterion by identity.

The default and explicit `all` Make selections attempt all four groups, even
after an earlier group fails, and return nonzero for any failure. Only base
runs the separate internal-package race selection. Every explicit group retains
format, dialect, vet, build, lint, security and vulnerability stages. Reuse the
existing acceptance runner, resource-selected worker count and literal argument
handling; add no new dependency or public factory command.

Keep the existing twenty-five-minute suite and thirty-minute job limits, race
instrumentation, assertions and ordered shared state. Each Ginkgo-driven group
keeps independent JSON/JUnit report paths and fail-on-empty behavior. The new
group uses the existing bounded base worker plan; its ordered family still runs
on one worker as required by Ginkgo's ordering contract.

## Hosted wiring

Extend the existing installation/handoff matrix with `source-cutover` on both
Linux and macOS. Retain the existing job names for the prior groups and upload
the new group's actual reports even on failure. The ordinary base job must no
longer execute these scenarios. Keep the focused trusted-main review repair
PR #143 independent; this scheduling correction belongs to the cutover PR.

## Independent qualification

An independent Ginkgo/Gomega author must first observe the old Make/CI wiring
fail the new four-group contract. Check anchored family neighbors, explicit
selection, default/all coverage, failure propagation, worker/report operands,
quality stages and hosted matrix/report wiring. Discover the actual identity
sets and counts; do not infer counts or replace omitted checks with a manifest.

After implementation, run the scheduler criteria with race instrumentation and
the source-cutover scenarios themselves. Recheck changed shell/YAML, relevant
lint and citations. Fresh hosted base and source-cutover jobs must pass before
claiming the deadline defect corrected at that level. Local passes alone do
not establish hosted timing or close a release gate.

## Local results

The independent scheduler/workflow selection first reported `3 Passed | 9
Failed` against the old Make/CI wiring. After implementation it ran:

```sh
FACTORY_AGENT_ROLE=spec-writer GOTOOLCHAIN=local GOMAXPROCS=2 \
  FACTORY_CLI_TEST_RACE=1 GORACE=atexit_sleep_ms=0 \
  go test -race -count=1 -timeout=5m -v ./acceptance -ginkgo.no-color \
  -ginkgo.focus='Go source qualification scheduling|Go source qualification workflow partition' \
  -ginkgo.fail-on-empty \
  -ginkgo.json-report=/private/tmp/factory-source-cutover-partition-green.json
```

Observed: `Ran 12 of 3174 Specs in 45.850 seconds`,
`12 Passed | 0 Failed`. Actual native discovery established the disjoint,
exhaustive identity union `3174 = 3019 base + 103 installation + 44 handoff +
8 source-cutover`; the additional criterion checks propagation of the new
group's failure. Both discarded-failure and missing-matrix controls failed at
their intended assertions, then the restored selection passed both cases.

The actual source-cutover scenarios also ran separately with fresh count,
evaluator/CLI race instrumentation, a fifteen-minute local timeout, the exact
suite-prefixed focus and an absolute local JSON report destination. Observed:
`Ran 8 of 3174 Specs in 468.769 seconds`, `8 Passed | 0 Failed`. This real local
execution is separate from dry-run identity discovery and the retained prior
hosted outcomes. Whole-module pinned lint reported `0 issues.`; the qualified
actionlint invocation with the existing narrow cache-key exception exited zero.
Independent correctness, security and coverage reviews found no actionable
finding. Fresh hosted base and source-cutover timing remains pending.
