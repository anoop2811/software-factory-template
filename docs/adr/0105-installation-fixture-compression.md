# ADR 0105: reduce installation fixture compression work

Date: 2026-10-19
Status: accepted for implementation
Decision: 104

## Context

PR #130 is merged. The remaining CI timing audit identified repeated gzip work
in the independent installation-image acceptance fixtures. The aggregate-wire
criterion builds both a 321 MiB decompressed archive and a one-byte overflow.
An earlier temporary experiment indicated that fixture compression level is a
useful optimization; its child binaries were race-instrumented, unlike CI.
The successful staging helper also compresses the same entries a second time
solely to calculate the expected archive digest.

## Decision

Use standard-library `gzip.NewWriterLevel` with `gzip.BestSpeed` only when
constructing the aggregate-wire boundary fixture. Handle the constructor error.
The production packager and its archive format/compression remain unchanged.
The existing shared archive fixture encoder retains its current compression.
This API predates Go 1.27; this is not a new Go API migration.

For the successful staging helper, encode entries once and calculate the expected
SHA-256 from those test-owned bytes before writing or invoking the stager. Reuse
those bytes for the input archive. Keep the expected digest independent of CLI
output and subsequent file mutations. No global cache, shared mutable fixture
state, production helper import or new dependency is introduced.

Retain actual 321 MiB and 321 MiB + 1 decompressed streams, real compiled stager
execution, full tree comparison on success, and refusal with empty stdout and no
final destination on overflow. Retain all other size, digest, source identity,
authentication, non-execution and publication proofs and the criterion census.
Do not reduce physical fixture sizes, mock staging or relax deadlines.

## Validation and consequences

Capture a focused before/after timing under CI's race-instrumented test harness
with `FACTORY_CLI_TEST_RACE` unset. Distinguish test-body timings from BeforeAll
compilation and whole-suite times. Run the installation-image acceptance group
and repository checks, with Linux/macOS qualification in CI. An independent
review checks that the digest oracle predates execution and boundary assertions
remain effective. Do not add flaky elapsed-time thresholds to behavioral tests.

Larger compressed test inputs are acceptable within existing archive limits.
Local timings quantify only the measured test and host; hosted critical-path
savings require actual hosted runs. No installed runtime behavior changes.

## Authoritative sources (fetched 2026-10-19)

- https://pkg.go.dev/compress/gzip#NewWriterLevel — supported compression levels
  and constructor error contract.
- https://go.dev/doc/devel/release — Go 1.27.2 released 2026-10-08; the existing
  repository toolchain is retained.


## Implementation evidence

RAN on 2026-10-19 on darwin/arm64 with Go 1.27.2, Python 3.12.14,
`GORACE=atexit_sleep_ms=0`, harness race enabled and
`FACTORY_CLI_TEST_RACE` explicitly unset. One local serial before/after pair
passed both real boundary criteria. Both invocations used:

```sh
go test -race -count=1 -timeout=4m -v ./acceptance \
  '-ginkgo.focus=accepts the exact aggregate wire boundary including padding and rejects one byte more|accepts 64 MiB total payload and refuses another bounded blob' \
  -ginkgo.v -ginkgo.no-color -ginkgo.fail-on-empty
```

Each invocation additionally set `-ginkgo.json-report` to its `baseline-1.json`
or `candidate-1.json` in the retained local artifact directory
`/var/folders/83/yj7qqyt551xbbpvm54tqkcbw0000gn/T/factory-fixture-compression-ci-policy-wipmvcn8`.
Ginkgo It-node end-event durations exclude BeforeAll preparation:

| Criterion | Baseline | Candidate | Less test-body time |
| --- | ---: | ---: | ---: |
| 64 MiB payload and one-byte overflow | 10.389031375 s | 7.130917209 s | 31.36% |
| 321 MiB wire and one-byte overflow | 32.532960042 s | 10.052213542 s | 69.10% |

Whole focused suite: 46.389813958 s before, 20.495093416 s after.
Both logs reported `2 Passed | 0 Failed | 0 Pending | 2953 Skipped`.
These are focused local measurements, not a prediction of hosted savings.
Independent report comparison retained the same 2,955 criterion identities.

Additional RAN checks: `go vet ./...` exited 0;
`golangci-lint run --config packs/go/.golangci.yml ./...` returned `0 issues.`;
`bash packs/go/hooks/ginkgo-only-check.sh` returned
`ginkgo-only-check: all Go behavioral tests use Ginkgo/Gomega`.
Full image-group and Linux/macOS hosted qualification are pending at this
record's creation; their outcomes belong to the PR evidence.
