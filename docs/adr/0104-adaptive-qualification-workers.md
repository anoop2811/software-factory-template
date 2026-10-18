# ADR 0104: select bounded qualification workers from runner resources

Date: 2026-10-18
Status: accepted for implementation
Decision: 103

## Context

ADR 0102 introduced two Ginkgo workers and retained complete, fresh qualification.
The user requested more than two workers when runner resources permit it. The
factory already pins Go 1.27.2, the latest stable patch observed on 2026-10-18.
More workers multiply race-instrumented processes, child compilers and fixtures.
The handoff suite currently contains exactly two indivisible ordered groups.

## Decision

Use a repository-private Go helper under `internal/qualification/cmd/worker-count`.
It is not installed in adopters or added to the public factory CLI. Its selection
policy is shared Go code, with independent Ginkgo/Gomega acceptance and unit tests.
There are no new dependencies or shell implementations of the resource policy.

`GO_RUNTIME_TEST_PROCS` defaults to `auto`. Automatic base selection is the minimum
of the configured cap, CPU budget, and memory budget, with a floor of one worker.
The CPU budget reserves one effective CPU when more than one is available. The
memory budget reserves 2 GiB, then allows one worker per remaining 2 GiB. These
are conservative scheduling budgets, not guarantees about peak child-process RAM.
Unknown or unsupported resource information selects one worker and logs why.

`GO_RUNTIME_TEST_MAX_PROCS` defaults to 4 and accepts canonical decimal integers
from 1 through 32. An explicit `GO_RUNTIME_TEST_PROCS` accepts the same integers
only when at or below that cap. It intentionally overrides automatic resource
budgets for reproducibility and controlled experiments; it does not override the
handoff's two-group cap. Empty, signed, padded, malformed and out-of-range values
fail before qualification stages. Existing explicit values 1 and 2 remain valid.

The helper prints exactly two validated integers (base and handoff) on stdout and
a bounded explanation on stderr. Make reads this plan once, passes the selected
count explicitly to Ginkgo, and never interpolates worker settings into shell
code. Installation remains serial. Handoff uses min(base workers, 2), including
explicit requests, until its ordered groups are deliberately restructured.

Resource detection uses effective CPUs from Go's default quota/affinity-aware
runtime calculation, additionally respecting a lower explicit GOMAXPROCS value.
Darwin uses physical memory with the stated reserved headroom. Linux uses
MemAvailable and the remaining allowance of every visible ancestor cgroup-v2
memory limit. Unrecognized cgroup layouts, v1 memory controllers or unreadable
required limits select one rather than assume host RAM belongs to a container.
The probe is read-only and bounded. Unknown operating systems select one.
Go's CPU quota calculation inspects the leaf cgroup, not tighter ancestor CPU
quotas; CPU selection remains a scheduling heuristic with that documented limit.
An explicitly disabled runtime cgroup-CPU calculation falls back to one CPU.

CI uses automatic selection with a configurable repository-variable cap
`GO_RUNTIME_TEST_MAX_PROCS` (default 4), preserves all six OS/group jobs and check
names, race instrumentation, fresh test execution, full criterion selection,
finite deadlines, failure propagation, quality gates and JSON/JUnit artifacts.
Worker selection is visible in logs; actual worker counts remain in Ginkgo JSON.
No larger paid runners or reduced test corpus are introduced.

## Validation and consequences

Independent tests must observe RED before implementation; cover resource/cap
boundaries, malformed settings, unknown-resource fallback, cgroup ancestors,
literal settings crossing Make without evaluation, the two-group handoff limit,
and actual execution/reporting of at least three distinct worker processes.
The existing exact, disjoint root-criterion census remains authoritative and
must include any new acceptance criteria rather than silently reducing coverage.

Run focused race tests, the source policy/selection acceptance, lint/security and
repository gates; both Linux and macOS qualification must be observed in CI.
Compare timing against PR 129 but do not claim a speedup until it is measured.
Standard public Linux runners should permit three workers; the three-core,
7 GB macOS runner can remain at two. Increasing the cap cannot create additional
parallel work in the current two-group handoff suite.

## Authoritative sources (fetched 2026-10-18)

- https://go.dev/doc/devel/release — Go 1.27.2, released 2026-10-08.
- https://go.dev/doc/go1.25#runtime — quota-aware runtime defaults.
- https://pkg.go.dev/runtime#SetDefaultGOMAXPROCS — refresh the default CPU budget.
- https://docs.github.com/en/actions/reference/runners/github-hosted-runners — runner CPU/RAM tables.
- https://go.dev/src/runtime/cgroup_linux.go — leaf CPU quota calculation.
- https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html — visible ancestor memory limits.
- https://onsi.github.io/ginkgo/#spec-parallelization — worker processes and ordered groups.

## Implementation evidence

RAN on 2026-10-18 with Go 1.27.2 and the qualified Python 3.12.14 interpreter:

- Initial `make go-runtime-source-check GO_RUNTIME_TEST_GROUP=base GO_RUNTIME_TEST_PROCS=3`
  returned `GO_RUNTIME_TEST_PROCS must be 1 or 2` and Make exit 2 (behavioral RED).
- `go test -race -count=1 -v ./internal/qualification`:
  `38 Passed | 0 Failed | 0 Pending | 0 Skipped`; package `0.398s`.
- `go test -race -count=1 -timeout=4m ./acceptance -ginkgo.focus='Go source qualification' -ginkgo.no-color -ginkgo.fail-on-empty`:
  `12 Passed | 0 Failed`; package `42.515s`.
- `go test -race -count=1 -timeout=2m ./acceptance -ginkgo.focus='Go source qualification worker execution' -ginkgo.no-color -ginkgo.fail-on-empty`:
  `ok github.com/anoop2811/software-factory-template/acceptance 9.614s`.
- `go vet ./...`: exit 0; `golangci-lint run --config packs/go/.golangci.yml ./...`:
  `0 issues.`; `gosec ./internal/qualification/...`: `Issues : 0`.
- `govulncheck ./...`: `Your code is affected by 0 vulnerabilities.`

- Independent literal-argv regression: `go test -race -count=1 -timeout=3m -v ./acceptance -ginkgo.focus='refuses invalid worker selection' -ginkgo.v -ginkgo.no-color -ginkgo.fail-on-empty`:
  `1 Passed | 0 Failed`; package `8.746s`. A test-only overlay removing the two
  freeze declarations from its copied Makefile produced `1 Failed` at the
  forbidden witness assertion (package `5.636s`); tracked production was untouched.

The real worker proof retains exact-once receipts, JSON/JUnit outcomes and failure
propagation. The root census remains 2,955 criteria, partitioned 2,851 base,
78 installation and 26 handoff. Complete Linux/macOS CI qualification and timing
comparison are pending when this implementation record is written.
