# Go loop checkpoint storage and recovery invariants

[ADR-0074](../adr/0074-go-loop-checkpoint-storage.md) adds the second milestone
of the existing loop conversion package: storage and read-only resume assessment.
It preserves schema-1 checkpoint identities and the Python flock namespace while
sharing filesystem mechanics with the Go budget ledger.

The private compiled checkpoint boundary can read, select, durably roundtrip and
assess a supplied checkpoint context. Assessment does not authorize a launch or
prove the supplied fingerprints are fresh. Future controllers must gather stable
observations and repeat admission under lock. No automatic recovery, counter reset,
uncertainty clearing or paid retry is introduced.

Installed commands remain unchanged. Manual checks/resume, bounded iteration,
public argument compatibility, transactional delivery and legacy retirement are
still open. Candidate storage is qualified only for cooperating processes on a
local filesystem with flock, atomic rename and fsync semantics.

## Deliberate qualification boundaries

The existing JSON decoder permits at most 512 nested containers and 4300 integer
digits. Reads and encoded writes are limited to 32 MiB. Nonfinite values anywhere
are refused, including unknown fields Python may read but cannot durably rewrite.
Escaped surrogate identity and unknown finite values remain intact. Refusal never
repairs or deletes the checkpoint. Budget's stricter scalar JSON contract stays
unchanged.

## Evidence

The evaluator first exercised the compiled CLI against the immutable Python Store
before production implementation. RAN:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test ./acceptance -ginkgo.focus='G2 loop checkpoint core' -ginkgo.no-color -ginkgo.succinct -count=1 -v
FAIL! -- 0 Passed | 3 Failed | 0 Pending | 1180 Skipped
--- FAIL: TestAcceptance (1.78s)
FAIL	github.com/anoop2811/software-factory-template/acceptance	2.279s
```

All oracle calls succeeded; the absent read/status/roundtrip requests returned
code 2 instead of the expected success. Implementation followed this RED.
An independent regression also caught an encoder-capacity bypass before its
correction: embedding bytes.Buffer promoted WriteString past the bounded Write
method. The publication check still refused oversized data, but intermediate
encoding exceeded its limit. RAN:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test ./internal/loop -ginkgo.focus='Loop checkpoint encoded capacity' -ginkgo.no-color -ginkgo.succinct -count=1 -v
```

RED: one failure; GREEN after using a named buffer field: one pass.

Real compiled SIGINT/SIGTERM cases also failed before the input correction: after
a completed 4 MiB pipe-write handshake, the process did not exit within three
seconds while its writer remained open. Closing nonpollable inherited stdin
could itself block. Both cases passed after adapting pipe input for polling and
restoring the original descriptor flags. A fixture-only canonical-output length
assertion was corrected to allow compact outer JSON; stored output still has an
exact 32 MiB positive boundary including its newline.

RAN the final 92-case checkpoint matrix with the actual CLI race-instrumented:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer FACTORY_CLI_TEST_RACE=1 go test -race ./acceptance -ginkgo.focus='G2 loop checkpoint' -ginkgo.no-color -ginkgo.succinct -count=1 -v
--- PASS: TestAcceptance (48.76s)
ok  	github.com/anoop2811/software-factory-template/acceptance	50.135s
```

All 92 selected cases passed, without conditional skips. Excluded unrelated
Ginkgo cases are not part of that count. The fixtures cover all three harness
names without invoking native CLIs or models.

RAN the final 19 checkpoint collaborator cases with race detection:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test -race ./internal/loop -ginkgo.focus='Loop checkpoint' -ginkgo.no-color -ginkgo.succinct -count=1 -v
--- PASS: TestLoopSnapshot (0.16s)
ok  	github.com/anoop2811/software-factory-template/internal/loop	1.490s
```

These include both Python/Go lock acquisition orders, held-lock lifetime across
successive publications, pre/post-rename faults, owned-temp cleanup, inode
replacement, cancellation, closed/poisoned transactions and bounded read retries.
Six input-lifecycle race cases separately passed for initially blocking and
nonblocking descriptors on success, cancellation and setup failure, observing
restored flags and a usable original descriptor.

Independent security review reran those six input cases and both compiled signal
cases; correctness review compared checkpoint and resume semantics with the
immutable oracle. No actionable findings remained after correction.

RAN Linux-target static analysis (not Linux runtime qualification):

```text
rtk proxy env GOOS=linux GOARCH=amd64 /private/tmp/factory-quality-tools/golangci-lint run --config packs/go/.golangci.yml ./...
0 issues.
```

The existing golang.org/x/sys v0.46.0 dependency becomes direct for Unix input
helpers; its BSD-3-Clause license was inspected. No version or go.sum changes.
RAN the complete source gate with the locally qualified tool directory on PATH:

```text
PATH="/private/tmp/factory-quality-tools:$PATH" make go-runtime-source-check
ok  github.com/anoop2811/software-factory-template/acceptance 438.322s
ok  github.com/anoop2811/software-factory-template/internal/budget 8.139s
ok  github.com/anoop2811/software-factory-template/internal/budgetcmd 5.572s
ok  github.com/anoop2811/software-factory-template/internal/input 2.397s
ok  github.com/anoop2811/software-factory-template/internal/loop 7.106s
ok  github.com/anoop2811/software-factory-template/internal/native 2.699s
0 issues.
Issues : 0
No vulnerabilities found.
```

The command exited 0, including vet, full repository race tests, build, lint,
security analysis and dependency vulnerability checks. RAN mapped shell checks:

```text
rtk proxy scripts/hooks/diff-aware-check.sh 3f05afabc89bc50f1e1d84c2d9667c41ab90818a HEAD
selftest: 217 passed, 0 failed, 0 skipped
diff-aware-check: all 1 dispatched check(s) passed
```

Main-targeted GitHub checks do not run
their normal gate on a stacked feature-branch base; local evidence is separate
from GitHub CI, live native execution and installed-runtime qualification.
