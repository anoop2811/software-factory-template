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
