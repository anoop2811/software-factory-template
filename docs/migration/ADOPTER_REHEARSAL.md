# Installed doctor permission rehearsal

Observed 2026-10-25; correction contract: Decision 112 in
[the decision log](../DECISION_LOG.md). Detailed adopter-specific records remain
local. This document describes only the factory permission defect and its source
qualification. It does not claim successful published replacement qualification.

## Installed failure

An isolated adopter rehearsal using local source bb8815a identified a genuine
integration mismatch. A supported freshly initialized v0.1.6 installation upgraded
successfully and retained its required state. Its installed public doctor then
reported its own sourceable configuration bridge as non-executable, despite all
fourteen installed gate proofs passing.

The declared configuration library is a readable regular 0644 file, per
[ADR 0098](../adr/0098-whole-installation-upgrade-and-rollback.md).
`internal/installation/roles.go:49` generates that mode. Native doctor had applied
the executable check for invoked hooks to this library too. Existing source-only
fixtures chmod the library 0700 and did not exercise the installed mode.

## Correction and independent evidence

The diagnostic now requires all core entries to be regular and readable. It
requires execute access for invoked hooks, while treating the named configuration
library as sourced. Installation modes and proof execution remain unchanged.

The independent compiled-public-command criteria include healthy 0644,
missing/unreadable/non-regular libraries, missing/non-executable/unreadable/
non-regular invoked hooks, and an actual qualified installation's public doctor.
They also check preservation of the installed library's bytes, mode and inode.

```text
# Before production correction
Ran 9 of 3008 Specs in 149.658 seconds
FAIL! -- 5 Passed | 4 Failed

# After correction, including actual installed public doctor
Ran 9 of 3008 Specs in 148.215 seconds
SUCCESS! -- 9 Passed | 0 Failed

# All native doctor acceptance criteria
go test -race -count=1 -timeout=6m -v ./acceptance -ginkgo.focus='Native Go doctor' -ginkgo.no-color -ginkgo.fail-on-empty
Ran 60 of 3008 Specs in 127.811 seconds
SUCCESS! -- 60 Passed | 0 Failed

# Focused permissions after the lint-required switch refactor
go test -race -count=1 -timeout=3m -v ./acceptance -ginkgo.focus='Native Go doctor sourceable library permissions' -ginkgo.no-color -ginkgo.fail-on-empty
Ran 8 of 3008 Specs in 8.767 seconds
SUCCESS! -- 8 Passed | 0 Failed

go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run --config packs/go/.golangci.yml ./internal/doctorcmd ./acceptance
0 issues.

go vet ./internal/doctorcmd ./acceptance
exit 0; no output

# Repository-pinned security scanner, targeted doctor package
gosec ./internal/doctorcmd
Files: 4; Issues: 0
```

Three independent review lenses found no actionable correctness, security or
coverage issue. The scheduler criterion was observed failing its old 2999-case
oracle, then passing native disjoint/exhaustive discovery with
`3008 = 2866 base + 98 installation + 44 release-handoff`.

These are local source results. Exact-head hosted CI, independent advisory review
and a new signed candidate are still required. RC1 remains failed and v0.1.6
remains latest stable. No release gate is closed merely by this correction.


## Default-cutover local rehearsal, 2026-10-28/29 UTC

This later rehearsal used an isolated supported installation containing unchanged
Go/TypeScript application code from the operator-selected project. The original
project remained outside the mutation scope. Detailed application identities,
paths and logs remain local. The fixture was genuinely initialized by immutable
v0.1.6; no version marker or ownership catalog was fabricated.

The initial application baseline passed backend race tests, frontend tests,
types, lint and build. A complete committed local-source Go package then passed
ordinary preview/confirmation, with no application/configuration or Git
HEAD/index changes. Its retained legacy factory matched the before bytes, was
inert, and was ignored by Git. Compatible rollback completed before modern loop
history existed. Later rollback inspection preserved that new history as an
explicit incompatibility rather than discarding it to permit downgrade.

A temporary independent real-HTTP criterion required `Cache-Control: no-store`
while preserving the existing health endpoint's200/status behavior. It failed
before the separately authored header change, passed afterward, then failed on
the same assertion after the implementation was reset. The temporary test and
requirement were removed; all105 application/operator input hashes matched the
original baseline again.

Actual installed Go manual `loop plan`, `loop run` and `loop status` commands
completed for Codex, Claude and OpenCode. Each run executed the application's
full configured product checks and language gates. Checkpoints reported
`manual_passed`, zero model attempts and empty budget-run lists; no paid budget
history was created. This proves manual integration, not live paid native-CLI
implementation/reviewer execution.

### Final-source interruption and recovery

A separate genuine legacy fixture used factory source
`67c27b201a3818d70ca90ea3ac292b1ecd826c4d` as an explicit local-source package.
The owned installer was observed stopped with an applying journal, two applied
rows and a regular pending marker, then exited by SIGKILL. Durable evidence
survived. A fresh read-only recovery preview preserved311 filesystem nodes;
confirmation with its fresh digest and explicit quiescence completed recovery.
The resulting descriptor named that exact source revision, the journal was
committed, and pending was absent. The owned process group was subsequently
observed absent.

Installed checks observed:

```text
./factory selftest
selftest: 14 shipped gates proved, 0 failed

make product-check
exit 0
```

Doctor, metrics and report also exited0. Compiler/Python poisons were not invoked
by the installed selftest, metrics or report. The product check used its normal
Go/application dependencies and a separate disposable database: backend race
suites excluding the existing E2E suite, lint, types,12 frontend tests and build.
No browser/E2E success is claimed. Doctor retained warnings about the copied
operator's optional review credentials and the clone's unconfigured Git hook
path; they were not suppressed to produce a clean-looking report.

All105 application/operator files, Git HEAD and index remained unchanged after
recovery and product checks. All212 tracked files and Git state of the original
project also matched the independent before snapshot. The installed scaffold
cache contained exactly the metrics and review-workflow templates. All four
retired Python runtime paths were absent in this supported fixture. The legacy
factory backup remained exact, inert and ignored. Local builds did not advance
release-based retention. The owned disposable database/container/profile were
removed afterward; the original stopped service profile and Docker context
were preserved.

The rehearsal exposed incomplete low-level JSON `recovery_arguments`: exact
replay omitted required local trust and project inputs and returned usage status2.
The correction retains those operands through one shared builder. Independent
existing forward/reverse real-SIGKILL criteria replayed the emitted command with
no writes and the same proposal digest:2 passed with suite and CLI race
instrumentation. A separate four-row source/predecessor trust matrix was seen
failing under the broken-builder control and passing afterward; the complete
installation package passed33 criteria.

This is local-source execution evidence, not publisher authentication, minimum-OS
qualification of a later candidate, or final operator acceptance. The source
revision above precedes the recovery-guidance correction; its successful
transaction and that correction's independent replay evidence remain separately
identified. Exact-head source CI, completed review, the final remnant audit and
actual published RC4 qualification still govern release readiness.
