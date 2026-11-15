# Rebalance source qualification within the existing CI partitions

Status: implemented; local source qualification passed; hosted qualification pending.
Decision: 171. CI follow-up within PR #182; no new release gate.

## Observed problem

The macOS base acceptance job for head 03a8b31 selected 3109 of 3495
criteria with two workers and reached its existing 25-minute limit. Both workers
exited with status 2 before final reports were available. The empty JSON report
and captured-output loss do not identify an individual blocked criterion.
This is a suite-capacity and diagnostic limitation, not evidence of a particular
new workflow failure. The other 45 checks passed, including all 25 migration
criteria on Linux and macOS and fresh initialization on both systems.

The preceding merged PR's macOS base report recorded 1307.664 seconds, with
1298.06 and 1297.21 specification-seconds across its two workers. The twelve
source qualification controls consumed 315.18 combined specification-seconds.
The current macOS source-entrypoints qualification step passed in 304 seconds.
The seven new installed-review workflow criteria consumed 3.59 seconds in the
current passing Linux base report; their macOS cost is unavailable in the failed
report. Do not attribute the timeout to those new criteria without evidence.

Evidence: Go runs 43539623922 and 43604796616; failed job 132477751096;
source-entrypoints job and retained Ginkgo reports were read on 2026-11-14.
The diagnostic reproduction retains seed 1794667785, two workers and both
25-minute limits, with verbose progress and unintercepted output.

## Decision and boundary

Move these three complete qualification-control families into the existing
source-entrypoints partition, alongside the native source routes they qualify:

- Go source qualification scheduling: ten criteria.
- Go source qualification worker execution: one criterion.
- Go source qualification workflow partition: one criterion.

Extend the single shared source-family selector already used for source focus
and base exclusion by both source and prepared execution. The base count becomes
3097 and the source-entrypoints count becomes 127. The sixteen partitions still
form the same disjoint, exhaustive union of 3495 criteria.

Keep job names, job count, workers, ordering, fixture ownership, priorities,
assertions, package-quality gates and all existing deadlines unchanged. Preserve
the current source partition's serial criterion and qualify coexistence with the
moved controls. No compiler reuse, new cache, retry, cap increase or test deletion
is part of this correction. The R4-R7 release gates remain unchanged.

The observed source step plus all moved specification time sums to about 619
seconds before accounting for parallelism. That arithmetic supports available
capacity; it is not a measured future runtime or a guarantee of a particular
speed improvement. Record actual post-change results separately.

## Independent acceptance

Before changing Make, update the independent selector oracle to require all
three complete families and prove that the current selection fails it. Retain
exact-root, positive-child and lookalike/suffix exclusions. Pin the twelve
existing names and source files so exchanging equal counts cannot hide a lost
or misrouted criterion. Retain the negative empty-selection check, complete
union, subprocess failure controls and source/prepared command parity.

Then run the complete 127-criterion source partition and 3097-criterion base
partition under the existing race/worker/deadline policy. The final PR head must
pass the hosted Linux/macOS matrix and receive model review. Do not replace
these checks with a claimed speed improvement.

## Qualification

The unchanged local reproduction on darwin/arm64 selected 3109 of 3495
criteria, with the same seed, two race-enabled workers and 25-minute bounds.
It ended after `25m3.569906125s` with `Test Suite Failed`: the suite deadline
interrupted the scheduler criterion after 8.473 seconds and ordinary installer
fixture preparation after 86.672 seconds. No assertion failure preceded the
suite deadline. The subsequent Go watchdog panic occurred during JSON report
generation. This reproduces the lost-report mechanism without identifying an
individual hung test or proving the hosted job had identical active criteria.
The seven new installed-review criteria completed before that deadline.

Command: the existing base exclusion with `go run -mod=readonly
github.com/onsi/ginkgo/v2/ginkgo --race --procs=2 --seed=1794667785
--timeout=25m --keep-going --fail-on-empty --no-color
--output-interceptor-mode=none --poll-progress-after=60s
--poll-progress-interval=60s -vv --trace ./acceptance -- -test.count=1
-test.timeout=25m`, plus the retained JSON/JUnit report options. Raw evidence is
`.factory/ci-mode-evidence/installed-review/macos-base-diagnostic.log`.
The host also had an unrelated virtual machine using about four CPU cores;
this local run is diagnostic evidence, not a hosted performance benchmark.

Before the Make edit, the updated independent oracle ran with
`go test -race -count=1 -timeout=5m -v ./acceptance
'-ginkgo.focus=Go source qualification scheduling executes disjoint feature partitions while retaining every source quality stage'
-ginkgo.no-color -ginkgo.fail-on-empty`. It reported `Ran 1 of 3495 Specs in
10.166 seconds`, `0 Passed | 1 Failed`, exit 1: the prepared base selection
incorrectly included all six moved family root/child positives. Evidence is
`.factory/ci-mode-evidence/installed-review/rebalance-red.log`.

Independent review matched all twelve moved names and source files against the
prior report and recomputed the original 115-criterion digest from retained
pre-rebalance discovery. The digest covers sorted full criterion text and origin
basename, excluding line numbers and absolute paths. This preserves the prior
identities without making ordinary line movement fail the oracle.

The shared Make selector is updated. With the same qualified Go 1.27.4/Python
environment used for the preceding diagnostics, `make go-runtime-source-check
GO_RUNTIME_TEST_GROUP=source-entrypoints GO_RUNTIME_TEST_PROCS=2
GO_RUNTIME_REPORT_DIR=.factory/ci-mode-evidence/installed-review/rebalanced-source`
exited 0. Output: `Ran 127 of 3495 Specs in 607.259 seconds`;
`SUCCESS! -- 127 Passed | 0 Failed | 0 Pending | 3368 Skipped`;
`0 issues.`; `Issues : 0`; `Your code is affected by 0 vulnerabilities.`
This includes the complete native discovery union, original identity digest,
prepared/source parity and the source group's existing serial criterion.
Evidence is `.factory/ci-mode-evidence/installed-review/rebalanced-source.log`
and its JSON/JUnit reports. No individual criterion was added or removed.

Final hosted qualification of the complete 3097-criterion base and
127-criterion source partitions on Linux/macOS, plus the remaining unchanged
matrix and model review, remains required. The local source result is not a
claim that the final hosted base timing has already improved.

Hosted head `ce47772` subsequently passed all 127 source criteria on Linux in
248.359 seconds and macOS in 354.870 seconds (Go run `43619019027`, jobs
`132521677107` and `132521677372`). The macOS base selected 3097 criteria and
completed inside the existing bound in 1154.223 seconds, but reported one
installed-review provider-fixture failure; 25 selected criteria had existing
platform/capability skips. This establishes completed bounded execution, not
a passing base gate. The provider failure and diagnostic follow-up are recorded
in [ADR 0154](0154-installed-review-workflow-migration.md). Exact-head acceptance
still requires all checks to pass. Model review for this head completed in
2m37s with all 31 files and 73 fragments covered, and all nineteen notes were
answered after full-source verification.

## Reproduce the retained source identity digest

The original 115 identities can be recovered from the full source gate's JSON
report by excluding the twelve explicitly moved qualification controls. Ginkgo
`SpecReport.FullText()` joins the container texts and leaf text; it does not
include `SuiteDescription`. The discovery helper adds that suite description
only for selector comparisons and removes it before computing this digest.

After the source command above, the following executed command printed
`bc4ae7453e4677c35774597eff99ef8293e08181828166cf46acc410354d6738  -`:

```sh
jq -r '.[0].SpecReports[] | select(.State == "passed" and .LeafNodeType == "It") | select((.ContainerHierarchyTexts[0] | startswith("Go source qualification ")) | not) | ((.ContainerHierarchyTexts + [.LeafNodeText] | join(" ")) + " :: " + (.LeafNodeLocation.FileName | split("/") | last))' \
  .factory/ci-mode-evidence/installed-review/rebalanced-source/source-entrypoints.json \
  | LC_ALL=C sort | shasum -a 256
```

The same selection counted 115 rows. Sorting, newline termination and origin
basenames are part of the recipe; absolute paths and line numbers are excluded.
This reproduction uses the report generated by the checked-in source gate and
does not require the original temporary capture path.

## Discovery execution boundary correction

Head `f3cc857` failed the macOS source-entrypoints census in Go run
`43641630212`, job `132591046959`. The first native empty-selection control
exceeded its one-minute context. Its child Ginkgo seed `1794691120` records
suite startup at 2026-11-14 21:18:40 UTC; the parent reported the deadline at
21:18:40.920607 UTC. Compilation and startup had consumed essentially the
discovery budget before enumeration began. The complete partition finished
in 404.440 seconds with this one failure; this was not the 25-minute suite cap.

The helper currently invokes `go test -race` eighteen times: the negative empty
selection, the complete census and sixteen partitions. Each invocation charges
compilation to the same one-minute metadata-execution limit. Remove those nested
builds from discovery. Reuse the existing suite-owned `sourceCompilerHelperImage`,
captured byte-for-byte from the running evaluator before other nested builds can
replace its workspace artifact. Invoke that executable with literal test and
Ginkgo dry-run flags, the same selectors and a separate report path per call.
Do not select the executable's compiler-handshake modes or execute spec bodies.

This narrows the earlier no-reuse boundary only for metadata enumeration: it
adds no compiler cache, download, installed runtime reuse or product change.
Discovery inherits the running evaluator's instrumentation. Both configured
source and prepared gates remain race-enabled; an ad-hoc non-race invocation
does not become a race test through this helper. Preserve the one-minute child
limit, all suite deadlines, exit/status assertions, exact counts, negative empty
selection, complete identity union and original/moved identity checks.

Before the helper edit, retain the hosted failure and exercise a test-owned Go
wrapper that refuses only nested `go test`, while permitting normal fixture
setup builds. The old discovery path must fail; the corrected path must still
run real native Ginkgo and pass the same complete census without that compiler
invocation. Then qualify the full 127-criterion source partition and lint.
Do not use a retry or an increased timeout as the correction.

The unchanged evaluator with an initially empty Go build cache passed the
focused census: `Ran 1 of 3495 Specs in 189.973 seconds`, `1 Passed | 0 Failed`.
That local run did not reproduce the hosted deadline. Its fixture setup also
uses Go, so an initially empty cache does not imply an empty cache at discovery.

For the independent control, a test-owned `go` wrapper exits 79 only for the
literal first argument `test` and delegates all other commands to the qualified
compiler. The pre-change race evaluator failed at the first real discovery:
`Ran 1 of 3495 Specs in 79.441 seconds`, `0 Passed | 1 Failed`, with
`controlled refusal: discovery must use the current evaluator without nested go test`.
The corrected race evaluator passed under the same wrapper:
`Ran 1 of 3495 Specs in 118.643 seconds`, `1 Passed | 0 Failed`.
The actual empty-selection refusal, all 3495 identities, sixteen disjoint groups
and retained original/moved identities were asserted normally.

Both binaries were built with `go test -race -c -o <owned-evidence-path>
./acceptance`, then invoked from `acceptance/` with the wrapper prepended to PATH:

```sh
<owned-evaluator> -test.v -test.timeout=5m -ginkgo.no-color -ginkgo.fail-on-empty \
  '-ginkgo.focus=Go source qualification scheduling executes disjoint feature partitions while retaining every source quality stage'
```

Exact commands are retained in
`.factory/ci-mode-evidence/installed-review/discovery-commands.txt`, with logs
`discovery-no-build-{red,green}.log` in that same evidence directory;
the initial empty-cache result is `discovery-cold-red.log` despite its passing
outcome. These controls qualify compiler-independent enumeration, not a hosted
speed guarantee or a reproduced cold-cache failure.

The first full local source run crossed repeated host sleep intervals and
reported `126 Passed | 1 Failed`. The failure was the existing cold-compiler
cancellation fixture's readiness wait, not discovery. Its timestamps overlap
the macOS power log's sleep/wake events; the suite's 60017.588-second wall span
and runner's 12m57.356 elapsed result are not usable performance measurements.
Retain that report as `discovery-source` and the power events as
`discovery-host-sleep.log`. Do not alter that fixture or its deadlines based on
this interrupted run.

On 2026-11-15, the unchanged code was qualified with a temporary local idle-sleep
assertion and the same source gate:

```sh
caffeinate -i bash -c 'make go-runtime-source-check GO_RUNTIME_TEST_GROUP=source-entrypoints GO_RUNTIME_TEST_PROCS=2 GO_RUNTIME_REPORT_DIR=.factory/ci-mode-evidence/installed-review/discovery-source-awake > .factory/ci-mode-evidence/installed-review/discovery-source-awake.log 2>&1'
```

The complete command exited 0: `Ran 127 of 3495 Specs in 650.688 seconds`,
`SUCCESS! -- 127 Passed | 0 Failed`; `0 issues.` from golangci-lint,
`Issues : 0` from gosec and `Your code is affected by 0 vulnerabilities.` from
govulncheck. Discovery passed in 161.978 seconds; the previously interrupted
compiler-cancellation criterion passed in 13.841 seconds. These local timings
are not a hosted speed comparison. Final-head CI and model review remain
required; head `f3cc857` finished with 45 passing checks and only the original
macOS discovery failure.
