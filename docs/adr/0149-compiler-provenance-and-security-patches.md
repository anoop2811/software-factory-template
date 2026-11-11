# ADR 0149: separate compiler provenance from artifact reader identity

Status: implemented; local qualification recorded below; hosted qualification pending.
Decision: 165. Security prerequisite discovered during Decision 164 qualification.

## Observed blocker and scope

On 2026-11-11 UTC the configured source gate passed all fourteen ordinary-upgrade
criteria, then `govulncheck ./...` failed on `GO-2026-6859` in `net/http@go1.27.3`,
reporting `net/http@go1.27.4` as the correction. The official
[Go release history](https://go.dev/doc/devel/release), fetched 2026-11-11 UTC,
lists Go 1.27.4 released 2026-11-10. The official
[advisory JSON](https://vuln.go.dev/ID/GO-2026-6859.json), fetched the same day,
records publication at 2026-11-11T02:51:11Z and the corrected version.

Build current factory artifacts with exactly Go 1.27.4. Keep this security
prerequisite separate from transactional project-CI migration. Do not suppress
vulnerability checks, upgrade unrelated dependencies or change worker/time limits.
Do not relabel a Go 1.27.4 binary as having been built with Go 1.27.3.

## Producer and reader contracts

The producer retains an exact, independently checked compiler pin. Update the
runtime module, source-build compiler constant, security-tool module and active
CI/build/review workflow pins and cache keys consistently. Emitted `source.json`
reports the compiler actually required for that build.

Artifact readers must not equate the producer's compiler version with their own
baked-in compiler pin. That makes every security patch break historical
predecessor/recovery inspection and makes future patch handoff impossible. The
compiler field is provenance, not authority to execute a binary or own a path.

Use one strict source-identity decoder for staging and read-only inspection.
Require exactly the five existing string fields. Reject duplicate, missing,
unknown, non-string and trailing data. Revision, release version, target and kind
must still equal the independently requested values. Require `go_version` to be
bounded canonical stable Go syntax: `go1.MINOR.PATCH`, with decimal components,
no leading zeros except a single zero, and at most nine digits per component.
Reject whitespace, prerelease/development/custom suffixes and malformed prefixes.
The grammar recognizes provenance from old and future stable compilers without
a rolling allowlist. It does not claim that every such compiler is secure.

Keep attestation, exact archive identity, digest verification, closed inventory,
runtime admission, immutable generation reconstruction and no-execution rules
unchanged. Syntactically valid compiler text cannot bypass any of those checks.
Source build still refuses a compiler differing from the exact current pin.

## Existing immutable readers

An already published experimental Go reader still has its old equality check.
It can reject a newer bundle before handing off to the new executable; changing
the new reader cannot repair that immutable program. Document the existing fresh
authenticated installer route for those users. That installer authenticates and
executes the selected raw bootstrap before the new owner stages its bundle.
Do not claim direct old-reader-to-new-reader handoff has been retroactively fixed.

The stable v0.1.6 Bash migration path and experimental Go rebootstrap must be
distinguished in upgrade guidance. Keep the public stable default unchanged;
this prerequisite does not publish or qualify v0.2.0.

## Honest historical qualification

Historical setup that rebuilds immutable Go 1.27.3 source must explicitly use a
qualified Go 1.27.3 compiler and keep its metadata truthful. Provision that
historical compiler only for development qualification, alongside the current
Go 1.27.4 compiler. It is not shipped as an adopter prerequisite or used for
current release artifacts. Never modify frozen source to disguise the compiler.

Use an explicit historical compiler path with exact version verification in the
shared historical setup helper. Reject an absent or wrong compiler with an
actionable diagnostic. Keep build cancellation/environment isolation and checked
fixture process ownership. Do not change historical golden bytes or recipe hashes.

## Independent qualification

Before implementation, observe current readers refusing otherwise valid Go 1.27.4
or future compiler provenance, alongside a healthy Go 1.27.3 control. Keep
malformed metadata negative controls. Exercise both public staging and inspection owners,
not only a helper in isolation. Require valid historical/current/future stable
provenance to preserve all independent identity, digest and inventory checks.

Observe producer refusal of a wrong compiler and successful truthful metadata
with the exact current compiler. Exercise a genuine immutable predecessor with
its historical compiler and current consumer. Cover the existing authenticated
installer rebootstrap without promising behavior from immutable old readers.

Run affected Ginkgo/Gomega race criteria, configured lint/vet/security checks,
workflow wiring controls and independent review. Record exact toolchains and
command outputs. Passing behavioral tests does not turn a failed vulnerability
scan into a passing gate; final-head hosted qualification remains required.

## Initial scanner evidence

The downloaded `go1.27.4.darwin-arm64.tar.gz` matched the official download
catalog's SHA-256
`91a44a2b8a716600ad94a4f143e156116620db8828086a6e3602291be9bd0bb4`.
Its executable reported `go version go1.27.4 darwin/arm64`. In the private
prerequisite checkout, with that compiler first on `PATH`, `GOTOOLCHAIN=local`,
`GOENV=off` and `GOMAXPROCS=2`, `govulncheck ./...` exited 0 and reported:

```text
No vulnerabilities found.
Your code is affected by 0 vulnerabilities.
```

It also reported ten module-level advisories outside the reachable call set and
zero advisories in imported packages. This scanner result establishes the effect
of the corrected toolchain on the existing source; it does not yet qualify the
producer-pin changes, metadata-reader changes or historical handoff.

## Local qualification receipts

These commands ran in a disposable checkout on 2026-11-11 UTC with the qualified
Go 1.27.4 compiler first on `PATH`, `GOTOOLCHAIN=local`, `GOENV=off` and
`GOMAXPROCS=2`. Historical tests additionally used the genuine native Go 1.27.3
executable through `FACTORY_HISTORICAL_GO`. They do not establish hosted CI or
release readiness.

```sh
go test -race -count=1 -v -timeout=5m ./acceptance -ginkgo.focus='Go source qualification scheduling|Go source qualification workflow partition' -ginkgo.fail-on-empty -ginkgo.no-color
```

```text
Ran 11 of 3444 Specs in 174.439 seconds
SUCCESS! -- 11 Passed | 0 Failed | 0 Pending | 3433 Skipped
```

```sh
go test -race -count=1 -v -timeout=8m ./acceptance -ginkgo.focus='requires fresh authenticated installer rebootstrap' -ginkgo.fail-on-empty -ginkgo.no-color
```

```text
Ran 1 of 3444 Specs in 55.741 seconds
SUCCESS! -- 1 Passed | 0 Failed | 0 Pending | 3443 Skipped
```

That second criterion built immutable historical source with its original
compiler, observed the old reader refuse current metadata, then observed a clean
preview through the fresh authenticated bootstrap without an adopter compiler
or Python. It does not claim the old executable itself became compatible.

The workflow's module-drift control initially refused the intentional but
unstaged `go.mod` patch in its source fixture. Staging that intended patch made
the unchanged guard's healthy control pass in the eleven-case run above.

`./scripts/selftest/adversarial-review.sh` exited 0 with
`adversarial-review: 53 passed, 0 failed`. The changed workflow-pin expectation
had first failed with 52 passes and one failure.
`./scripts/hooks/diff-aware-check.sh` exited 0 with
`selftest: 218 passed, 0 failed, 0 skipped` and
`diff-aware-check: all 1 dispatched check(s) passed`.
`go vet ./...` exited 0 without output. Final-source `govulncheck ./...` exited 0
with `No vulnerabilities found.` and `Your code is affected by 0 vulnerabilities.`
It again reported zero imported-package advisories and ten module-only advisories
outside the reachable call set.

An oversized local serial aggregate of five artifact/historical families reached
its fifteen-minute test-process timeout. It is not recorded as a passing suite.
The exact remaining 34 criteria then ran with the existing two-worker allowance:

```sh
go run -mod=readonly github.com/onsi/ginkgo/v2/ginkgo --procs=2 --race --timeout=25m --keep-going --fail-on-empty --no-color --focus='G1 committed-source runtime packaging|Qualified obsolete initializer cache retirement|Supported predecessor recipe compatibility (refuses each|preserves an edited|guards both|minimal qualified initializer inventory)' --json-report=.factory/go1274-evidence/artifact-remaining-green.json ./acceptance -- -test.count=1 -test.timeout=25m
```

```text
Ran 34 of 3444 Specs in 803.308 seconds
SUCCESS! -- 34 Passed | 0 Failed | 0 Pending | 3410 Skipped
Ginkgo ran 1 suite in 13m26.098431959s
Test Suite Passed
```

This local command did not alter production workflow worker or timeout limits.

`golangci-lint run --config packs/go/.golangci.yml ./...` exited 0 with
`0 issues.`; `gosec ./...` exited 0 with `Files : 260` and `Issues : 0`.
The configured lint initially reported G702 at the test process helper because
the historical compiler comes from explicit developer/CI executable selection.
Independent security review confirmed that this input is trusted configuration,
not artifact data. Its execution uses separate arguments without a shell. A
single existing `#nosec` annotation now names G204/G702 and that rationale;
there is no global scanner exclusion. Exact version/build-information checks
establish historical reproducibility, not trust in an arbitrary executable.

Finally, the complete 93-case strict-reader matrix and three producer criteria
passed together after the annotation, separately from workflow setup:

```sh
go test -race -count=1 -v -timeout=5m ./acceptance -ginkgo.focus='Compiler provenance across artifact readers|publishes a native bundle with exact metadata|refuses a compiler whose version differs from the pin|accepts the producer installation flag' -ginkgo.fail-on-empty -ginkgo.no-color -ginkgo.json-report=../.factory/go1274-evidence/metadata-producer-green.json
```

```text
Ran 96 of 3444 Specs in 19.113 seconds
SUCCESS! -- 96 Passed | 0 Failed | 0 Pending | 3348 Skipped
```

Before implementation, the healthy old-compiler metadata controls passed while
six current/future-provenance cases failed, and all three producer criteria
failed against the old pin. The unchanged malformed/identity controls remain
part of the passing reader matrix.

## Hosted qualification follow-up

PR #179 head `72515fb` passed the artifact, historical-recipe and other runtime
partitions, but both source-entrypoint jobs panicked while decoding a language-pack
workflow: `duplicated key 'shell' in struct acceptance_test.step`. Adding `Shell`
to the shared workflow-step oracle collided with an existing inline wrapper's
own `Shell` field. This is a test-model composition error, not invalid workflow
YAML or a runtime/compiler failure.

Keep one declaration for the YAML field and retain the existing distinction
between absent shell configuration and an explicit shell override. Reproduce
the actual structural-workflow criterion before correction, then rerun both its
source-entrypoint partition and the compiler-workflow criteria that share the
type. Keep all workflow execution, shell-absence and historical compiler wiring
assertions; do not skip the criterion or relax its checks.

The exact structural-workflow criterion reproduced the hosted duplicate-field
panic locally before correction. After using one optional shared `Shell` field,
the structural consumer and historical compiler workflow consumer passed together:

```sh
go test -race -count=1 -v -timeout=5m ./acceptance -ginkgo.focus='executes the actual structural steps from every language-pack workflow with a native runtime|Go source qualification workflow partition' -ginkgo.fail-on-empty -ginkgo.no-color -ginkgo.json-report=../.factory/security-prereq-evidence/structural-shell-workflow-green.json
```

```text
Ran 2 of 3444 Specs in 91.099 seconds
SUCCESS! -- 2 Passed | 0 Failed | 0 Pending | 3442 Skipped
```

This focused result is not a claim that the entire source-entrypoint partition
or the corrected hosted head has passed. Both still require their own evidence.

The corrected head's Linux source-entrypoint job passed. Its macOS counterpart
passed 114 of 115 criteria and exposed a different mock-evaluator failure: the
test tried to read `mock-current.json` before checking the command's exit status
and diagnostics. Preserve all existing expected outcomes, but assert the process
result before reading its artifacts so a refusal cannot be reported merely as
a missing file. Keep scoring, oracle, baseline and ownership assertions intact.
Investigate the underlying startup behavior with that evidence; do not substitute
retries, omitted checks or an arbitrary deadline increase for a diagnosis.

The diagnostic-only correction passed its focused criterion:

```sh
go test -race -count=1 -v -timeout=2m ./acceptance -ginkgo.focus='scores pass, fail and oracle tampering through the converted mock adapter' -ginkgo.fail-on-empty -ginkgo.no-color -ginkgo.json-report=../.factory/security-prereq-evidence/mock-status-diagnostic.json
```

```text
Ran 1 of 3444 Specs in 3.943 seconds
SUCCESS! -- 1 Passed | 0 Failed | 0 Pending | 3443 Skipped
```

`golangci-lint run --config packs/go/.golangci.yml ./...` also exited 0 with
`0 issues.`. Neither result establishes a production-runtime correction.

Local full-group attempts remain nonpassing evidence: two four-worker runs
reported startup-readiness failures. A subsequent two-worker run passed 114 of
115 criteria, but its failed cold-source readiness observation crossed a recorded
322-second host Idle Sleep, from 2026-11-11 12:19:11 to 12:24:33 -0700. That run
cannot establish a clean qualification result. The sleep observation does not
explain the earlier four-worker failures or the hosted macOS mock failure.

The corrected hosted head also failed its macOS release-handoff job. Its detailed
report is still required to identify the cause; elapsed job time alone is not a
diagnosis. Keep these open outcomes separate from the resolved YAML panic and
the passing focused diagnostic check. No production deadline, retry policy or
hosted worker limit is changed by this follow-up.
