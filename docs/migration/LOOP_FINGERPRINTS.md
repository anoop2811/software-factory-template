# Go loop fingerprint foundation

[ADR-0073](../adr/0073-go-loop-fingerprint-foundation.md) introduces a private,
read-only compiled fingerprint boundary. It establishes the identity required
for future loop checkpoint freshness; it does not execute or resume a loop.

The oracle is scripts/lib/loop.py at commit
76952eaa63aebd1ecd282f5ab51dd7c3627cb497. Snapshot and policy digest strings
are compared exactly, including Python canonical JSON, numeric identity and
filesystem surrogateescape. The Go implementation must not call Python.

## Qualification boundaries

Git and system grep remain external prerequisites, preserving inherited
environment/configuration and POSIX ERE behavior. The private source scanner
refuses more than 100000 entries, 64 MiB per source file, 512 MiB total source
bytes or 32 MiB probe output. These extra bounds are recorded qualification
differences requiring review before activation. Native-policy baseline limits
remain 512 entries/files, 1 MiB per file and 8 MiB total.

The snapshot is not an atomic filesystem transaction. Two observations detect
changes between them; arbitrary blocking filesystem syscalls and hostile
same-user replacement transactions are not claimed safe. The implementation
must refuse uncertain observations without returning partial usable identity.

Checkpoint storage/recovery, manual checks/resume, bounded model iteration,
public argument routing and installed migration/cleanup remain pending.

## Evidence

Independent Ginkgo/Gomega acceptance invokes the compiled CLI and the immutable
Python oracle in isolated real Git repositories. No native harness or paid model
is invoked. Before implementation, this command observed the missing operation:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test ./acceptance -ginkgo.focus='G2 loop fingerprint core' -ginkgo.no-color -ginkgo.succinct -count=1
FAIL! -- 0 Passed | 3 Failed | 0 Pending | 1085 Skipped
```

Additional independent RED evidence was recorded before each correction:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test ./acceptance -ginkgo.focus='G2 loop fingerprint signaled grep' -ginkgo.no-color -ginkgo.succinct -count=1 -v
FAIL! -- 0 Passed | 2 Failed | 0 Pending | 1169 Skipped

rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test ./acceptance -ginkgo.focus='G2 loop fingerprint process signals' -ginkgo.no-color -ginkgo.succinct -count=1 -v
FAIL! -- 0 Passed | 2 Failed | 0 Pending | 1172 Skipped

rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test ./acceptance -ginkgo.focus='invalid range with retained literal' -ginkgo.no-color -ginkgo.succinct -count=1 -v
FAIL! -- 0 Passed | 1 Failed | 0 Pending | 1174 Skipped
```

These exposed a signaled grep being mistaken for a successful/nonmatching probe,
OS interruption leaving an owned Git probe alive, and protected character-class
normalization changing the safety digest. The expanded first run also exposed
private extra operands in diagnostics; its rejection case now requires sanitized
stderr and no identity output.

Two raw non-UTF8 filename cases are conditional on the fixture filesystem. APFS
on this development machine rejects those names with EILSEQ, so they are skipped
here. Raw-byte environment values and keys, non-BMP keys and U+FFFD identity are
covered separately without relying on raw filename support. A local pass does
not qualify the skipped filename cases or Linux behavior.

RAN the final race-instrumented compiled CLI matrix after review corrections and
fixture lint cleanup: 93 passed, two filesystem skips (95 selected). The focused
protected-glob and OS-signal correction run also passed all 15 cases before the
complete matrix rerun. The core suite's unrelated specs are
excluded by the focus expression, not counted as qualification.

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer FACTORY_CLI_TEST_RACE=1 go test -race ./acceptance -ginkgo.focus='G2 loop fingerprint' -ginkgo.no-color -ginkgo.succinct -count=1 -v
--- PASS: TestAcceptance (54.24s)
ok  	github.com/anoop2811/software-factory-template/acceptance	55.592s

rtk proxy env FACTORY_AGENT_ROLE=spec-writer FACTORY_CLI_TEST_RACE=1 go test -race ./acceptance -ginkgo.focus='Python protected glob semantics|G2 loop fingerprint process signals' -ginkgo.no-color -ginkgo.succinct -count=1 -v
--- PASS: TestAcceptance (10.25s)
ok  	github.com/anoop2811/software-factory-template/acceptance	11.594s

rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test -race ./internal/loop -ginkgo.no-color -ginkgo.succinct -count=1 -v
--- PASS: TestLoopSnapshot (3.33s)
ok  	github.com/anoop2811/software-factory-template/internal/loop	4.801s
```

The 16 internal cases include actual read growth, FIFO/symlink substitution,
stable double-observation, configuration-field coverage, probe limits and
post-start cancellation. The cancellation fixture waits for a child PID before
canceling; fixed startup latency is not evidence of child cleanup.

Independent correctness and security review found no remaining actionable
findings after correction. The security reviewer also rebuilt the command and
observed both interrupted probe PIDs absent, code 2 and empty stdout.

RAN `make go-runtime-source-check` with
`/private/tmp/factory-quality-tools` prepended to PATH. Shell syntax, formatting,
Ginkgo policy, vet, the complete race suite, build and lint passed. Exact excerpts:

```text
go test -race -count=1 ./...
ok  	github.com/anoop2811/software-factory-template/acceptance	422.922s
ok  	github.com/anoop2811/software-factory-template/internal/budget	7.527s
ok  	github.com/anoop2811/software-factory-template/internal/budgetcmd	5.699s
ok  	github.com/anoop2811/software-factory-template/internal/loop	6.224s
ok  	github.com/anoop2811/software-factory-template/internal/native	2.198s
golangci-lint run --config packs/go/.golangci.yml ./...
0 issues.
```

That invocation stopped at standalone gosec's G304 warning on the shared open.
Independent security review confirmed that caller-selected external config is
intentional, so checkout confinement would break the contract. A sink-local
annotation records the read-only, nofollow/nonblock, identity and byte-bound
checks. This correction changed comments only. RAN the affected checks again:

```text
rtk proxy env FACTORY_AGENT_ROLE=implementer /private/tmp/factory-quality-tools/gosec ./...
  Files  : 58
  Lines  : 8794
  Nosec  : 14
  Issues : 0

rtk proxy env FACTORY_AGENT_ROLE=implementer /private/tmp/factory-quality-tools/golangci-lint run --config packs/go/.golangci.yml ./...
0 issues.
```

RAN the remaining vulnerability check:

```text
rtk proxy /private/tmp/factory-quality-tools/govulncheck ./...
No vulnerabilities found.
```

The complete make command was not rerun
after this comment-only correction; the results above identify the executed
components rather than asserting an unobserved aggregate exit status.

RAN the mapped shell gate on the implementation commit:

```text
rtk proxy scripts/hooks/diff-aware-check.sh 6f91b97990a21595a5d19ddf6d4d01acd090164c HEAD
selftest: 217 passed, 0 failed, 0 skipped
diff-aware-check: all 1 dispatched check(s) passed
```

Decision-log and commit-message gates passed. Citation lint explicitly skipped
because no citation prefix is configured; source references were resolved
against the actual files. Main-targeted GitHub workflows do not run their normal
checks on this stacked feature-branch PR; local results do not constitute GitHub
CI or default-release qualification.
