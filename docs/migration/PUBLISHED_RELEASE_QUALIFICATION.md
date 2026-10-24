# Qualify a published Go release candidate

The contract is [ADR 0107](../adr/0107-published-release-qualification.md).
This repository-only lane exercises actual published assets and their GitHub
attestations. Regular users continue to use the single `factory` CLI.
Adding the lane does not establish that a release passed it. The
[replacement-release checklist](GO_RELEASE_READINESS.md) remains authoritative.

## First published candidate result

On 2026-10-24, [v0.2.0-rc.1](https://github.com/anoop2811/software-factory-template/releases/tag/v0.2.0-rc.1)
was published at `5ef20c186676069229cae3781c85ec509f377af2`.
[Release run 40103396983](https://github.com/anoop2811/software-factory-template/actions/runs/40103396983)
passed four native builds, attestation/upload of all twelve subjects and the
qualification guard. The release contains fourteen assets including the shared
attestation bundle and checksum file. It remains a prerelease; v0.1.6 is latest
stable.

Actual published qualification **failed on all four targets**. Each evaluator
first authenticated its three native subjects with the real verifier, then its
five init/upgrade/rollback/recovery criteria failed in candidate staging. The
consumer copied the attestation bundle into private custody under an extensionless
name; the real GitHub CLI refused it with `bundle file extension not supported,
must be json or jsonl`. This is a consumer failure, not an artifact-signature
failure or successful migration evidence. The release notes mark RC1 unsuitable
for migration. Its signed source, tag and assets are retained unchanged.

| Target | Observed OS | Published scenarios |
|---|---|---|
| Linux amd64 | Ubuntu 24.04.5 LTS | 0 passed, 5 failed |
| Linux arm64 | Ubuntu 24.04.5 LTS | 0 passed, 5 failed |
| macOS amd64 | 15.7.9 | 0 passed, 5 failed |
| macOS arm64 | 14.8.9 | 0 passed, 5 failed |

The retained `published-release-TARGET-40103396983-1` artifacts contain the exact
subject digests, source/harness identity, OS and Ginkgo failure messages. Decision
110 records the correction contract. A corrected source implementation and local
stager checks do not repair RC1 or close the real release gate: publish a new
candidate after review and repeat the full live matrix. The macOS 14 Intel gap,
mandatory adopter pilot and default cutover remain separate requirements.

## Local correction evidence

Decision 110 preserves the original bundle format suffix in the private snapshot
used by the generic stager. Independent public regressions first observed two
failures for JSON/JSONL and one existing unsupported-format refusal pass. After
the correction, the same three criteria passed. They exercise a compiled consumer
with GitHub-compatible suffix dispatch, private permissions, unchanged bytes after
caller-input mutation, staged payload identity and refusal without publication.
The source census is now 2,999 ordinary criteria; no previous criterion was removed.

Observed 2026-10-24 with Go 1.27.3:

```text
go test -race -count=1 -timeout=6m -v ./acceptance -ginkgo.focus='G1 runtime bundle staging|G1 complete installation source images' -ginkgo.no-color -ginkgo.fail-on-empty
Ran 124 of 2999 Specs in 142.593 seconds
SUCCESS! -- 124 Passed | 0 Failed | 0 Pending | 2875 Skipped
ok github.com/anoop2811/software-factory-template/acceptance 143.220s

go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run --config packs/go/.golangci.yml ./internal/staging ./acceptance
0 issues.

go vet ./internal/staging ./acceptance
exit 0; no output
```

A separate local real-verifier probe used RC1's actual signed Darwin arm64
installation archive, its release bundle, freshly provisioned trusted roots,
exact source `5ef20c186676069229cae3781c85ec509f377af2` and tag ref. Real
`gh attestation verify` rejected the unchanged bundle copied to an extensionless
filename with exit 1 and accepted its `.jsonl` name with exit 0. The corrected
public `factory-stage --installation` then authenticated and staged that same
archive with both the JSONL bundle and a pretty-printed single-bundle JSON form:
each exited 0 and reported `authentication: github-attestation`.

These observations qualify corrected local staging, not a new signed Go runtime
or completed public migration. RC1 remains failed; a reviewed correction must be
published under a new candidate tag and pass the full live matrix. No signature,
source, private-custody, process-ownership or extraction check was relaxed.

The first correction PR run also exposed the scheduler's frozen 2,996-case
expectation after adding the three format criteria. Updating only the active
census expectations retains exhaustive/disjoint membership and all quality-stage
checks. Independent native discovery passed the focused scheduler criterion:
`1 Passed | 0 Failed` in 19.544 seconds, with
`2999 = 2858 base + 97 installation + 44 release-handoff` and zero-selection refusal.

The permission fixtures were subsequently strengthened to use caller directory
0755 and bundle 0644 while still requiring private snapshot directory 0700 and
file 0600. A temporary Go compiler overlay changing only private destination
creation from 0600 to 0644 produced `1 Passed | 2 Failed`, with both valid-format
cases rejected as `verification snapshot is not private`. Unmodified production
then produced `3 Passed | 0 Failed` in 1.661 seconds. No tracked production file
was changed by the mutation probe, and no extra criterion was added.

## Select an exact release

Use a non-draft official release or prerelease containing the complete native
runtime, installation image, bootstrap and attestation assets. Choose its full
reviewed source commit independently; do not substitute a moving `main`, `latest`
or a newly resolved tag for the expected revision.

After the qualification workflow is merged, the existing release workflow calls
it only after successful build, attestation, verification and upload. A published
release event alone is too early because its assets may not yet exist.

To requalify an existing published tag from the trusted default branch:

```sh
gh workflow run published-release-qualification.yml --ref main \
  -f tag=TAG -f revision=FULL_REVIEWED_SOURCE_SHA
```

Replace both placeholders. A draft release, mismatched or moved tag, malformed
operand, wrong repository or untrusted dispatch ref is refused before candidate
execution. The guard also requires all twelve native assets and the shared
attestation bundle before scheduling the matrix. Presence does not grant trust.
The independent real GitHub verifier authenticates the exact native
subjects, source commit, tag ref and signer workflow before downloaded code runs.
The public installer then repeats its own supported acquisition checks.

## Read the evidence

The native matrix covers Linux amd64/arm64 and macOS amd64/arm64. It runs five
live criteria per target: fresh initialization, both genuinely initialized Bash
predecessors with migration/rollback, and both predecessors with witnessed
interruption/recovery. Go/Python and model clients are unavailable to child Go
factory processes. The harness can still use development tools, and checking a
restored Bash predecessor can require its original tools.

The workflow has contents-read permission. Signing and release-upload privileges
remain in the preceding attestation job. Live qualification makes no paid model
calls. Its finite deadlines and four-target matrix bound the requested CI work;
ordinary PR/source runs do not perform release downloads.

Find the run in Actions and download its artifacts:

```sh
gh run list --workflow published-release-qualification.yml
gh run download RUN_ID --pattern 'published-release-*' --dir qualification-evidence
```

A call inside the release workflow belongs to that release's Actions run. A
manual dispatch has its own qualification run. Each native artifact contains
Ginkgo JSON/JUnit outcomes and structured evidence binding the release, trusted
harness commit, exact authenticated subjects and digests, actual OS/architecture,
scenario outcomes and workflow identity. Inspect failed-run diagnostics as well
as successful outcomes. A missing artifact or skipped scenario cannot close a
release gate.

## Keep minimum-OS evidence separate

The approved floor remains Ubuntu 24.04+ and macOS 14+ on both architectures.
The default Intel Mac runner is macOS 15; a pass there does not prove macOS 14
Intel compatibility. Record that outstanding native minimum-OS evidence rather
than marking the full platform gate complete. GitHub's announced retirement of
hosted macOS 14 on 2026-11-02 also requires an explicit qualification plan; this
lane does not silently raise the supported floor or provision paid runners.

[GitHub's macOS 14 notice](https://github.blog/changelog/2026-10-01-github-actions-macos-14-runner-image-retirement/)
was read on 2026-10-19 UTC.

## Local contract checks and live execution

The ordinary Ginkgo source suite executes workflow graph, guard/refusal and
failure-propagation controls without network access. The live file uses the
`publishedrelease` build tag and is not compiled into ordinary source acceptance.
A live invocation requires `FACTORY_PUBLISHED_TAG`,
`FACTORY_PUBLISHED_REVISION` and `FACTORY_PUBLISHED_EVIDENCE_DIR`; missing inputs
fail rather than producing a green environment skip. See the checked-in workflow
for the exact invocation and report flags.

Keep implementation, PR qualification, actual published-run outcomes, minimum-OS
coverage, adopter pilot, default Go selection and stable publication separate.
A successful lane run does not itself complete the required adopter pilot or
change existing source/Bash defaults.

CLI references read 2026-10-19 UTC:
[workflow run](https://cli.github.com/manual/gh_workflow_run),
[run download](https://cli.github.com/manual/gh_run_download), and
[attestation verify](https://cli.github.com/manual/gh_attestation_verify).

## Source contract evidence

Observed 2026-10-19 UTC using the existing qualified Go 1.27.2/Python 3.12.14
harness environment, with `FACTORY_CLI_TEST_RACE` unset for child executables:

```text
go test -race -count=1 -timeout=4m -v ./acceptance -ginkgo.focus='Go source qualification|Published release qualification workflow contract' -ginkgo.no-color -ginkgo.fail-on-empty
Ran 16 of 2996 Specs in 60.939 seconds
SUCCESS! -- 16 Passed | 0 Failed | 0 Pending | 2980 Skipped
ok github.com/anoop2811/software-factory-template/acceptance 61.752s
```

The independent initial RED was a missing qualification workflow. The source
census is now 2,855 base + 97 installation + 44 release-handoff criteria. These
sixteen focused checks include actual five-criterion tagged discovery, a Go
overlay removing the live registration (zero selection must fail), missing-input
refusal before network access, executed workflow guards and runner failure
propagation. Discovery does not execute the live scenario bodies.

Additional commands observed in this session:

```text
./scripts/selftest/run.sh
selftest: 217 passed, 0 failed, 0 skipped

golangci-lint run --build-tags=publishedrelease --config packs/go/.golangci.yml ./...
0 issues.

go vet -tags=publishedrelease ./...
exit 0; no output

go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -color
exit 0; no output
```

The first submitted source head was
[935d58a](https://github.com/anoop2811/software-factory-template/commit/935d58aab26df5fbb913e4d8121f36295cd8b505).
Its [Go source CI run](https://github.com/anoop2811/software-factory-template/actions/runs/39339436829)
passed on both operating systems. Later PR revisions have separate exact-head
check results; these dated command outputs are historical observations.

Independent correctness, security and test review plus finding verification found
no surviving source finding. The five real published-release scenarios remain
NOT RUN; neither this evidence nor a source-CI pass closes that release gate.


Review follow-up observed 2026-10-20 UTC in the same qualified environment:

```text
go test -race -count=1 -timeout=2m -v ./acceptance -ginkgo.focus='Published release qualification workflow contract' -ginkgo.no-color -ginkgo.fail-on-empty
Ran 4 of 2996 Specs in 18.530 seconds
SUCCESS! -- 4 Passed | 0 Failed | 0 Pending | 2992 Skipped
ok github.com/anoop2811/software-factory-template/acceptance 19.170s
```

Each of the thirteen required release assets is independently omitted by a
negative control. A missing-bundle RED preceded early guard wiring. Synthetic
child receipts confirm inherited host tools/home/temp/read-token behavior,
explicit candidate overrides, and isolation of offline fixtures from ambient
GitHub/Actions values; the isolation control also observed RED before correction.
These additional controls remain inside the same four ordinary criteria, so the
source census is unchanged. Actual published qualification remains NOT RUN.


Before publishing an RC, refresh the qualified toolchain against upstream.
On 2026-10-20 UTC, [Go's release history](https://go.dev/doc/devel/release)
confirmed Go 1.27.2 exists (2026-10-08) and Go 1.27.3 was released on 2026-10-19
with a crypto/x509 security fix. The initial lane reused the then-qualified
1.27.2 pin. The subsequent source run at `4450120` failed its mandatory scan on
reachable [GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809), so
[Decision 107](../DECISION_LOG.md)
moves the active compiler, metadata contract, workflows and exact tool-cache keys
to 1.27.3 before current qualification can pass. Earlier command outputs retain
their actual toolchain and source identity; they are not patch qualification.
Previously unreleased Go candidates require repackaging under the existing exact
compiler policy. Immutable Bash/v0.1.6 references and adopter language packs stay
unchanged. Record patched source results separately from actual published runs.

Patch qualification observed 2026-10-22 UTC with Go 1.27.3 and the existing
Python 3.12.14 harness, with `FACTORY_CLI_TEST_RACE` unset:

```text
go test -race -count=1 -timeout=2m -v ./acceptance -ginkgo.focus='Go source qualification workflow partition' -ginkgo.no-color -ginkgo.fail-on-empty
1 Passed, 0 Failed; package 9.823s

go test -race -count=1 -timeout=6m -v ./acceptance -ginkgo.focus='G1 committed-source runtime packaging|G1 runtime bundle staging|G1 complete installation source images|Installed runtime root separation' -ginkgo.no-color -ginkgo.fail-on-empty -ginkgo.json-report=/private/tmp/factory-pr133-go1273-affected-acceptance.json
145 Passed, 0 Failed; package 151.321s

govulncheck ./...
No vulnerabilities found.
Your code is affected by 0 vulnerabilities.
```

The workflow expectation first failed against the old compiler cache key under
Go 1.27.2. The new JSON report independently records 20 packaging, 69 complete
image, 52 staging and four installed-root criteria passed. The scanner also
reported eight advisory entries in required modules that are not reachable from
the analyzed code; this is not a claim that every module is advisory-free.
These observations qualify local source behavior; patched hosted native checks
and actual published-release scenarios remain separately required.

## Source-CI scheduling correction

The macOS source-handoff job in [run 39364383333](https://github.com/anoop2811/software-factory-template/actions/runs/39364383333/job/118437939519)
failed its suite deadline after all 44 selected criteria passed. Its retained
report placed both longest ordered groups on one worker. The bounded correction
in docs/adr/0107-published-release-qualification.md:194 assigns three coarse
priorities while preserving those whole groups, all assertions and existing
worker/resource limits and deadlines. This does not change installed behavior.

The independent oracle first exposed evaluator setup issues: parallel dry-run
reporting, omitted fields in Ginkgo's custom JSON serializer, and a nested test
container that did not match the real four top-level groups. Those attempts are
not counted as a priority differential. After correcting the evaluator, a source
overlay removing only the three real decorators produced the expected scheduling
failure; the unmodified candidate passed. One generated bootstrap is shared
between the two separate fixture programs, retaining the repository dialect gate.

RAN on 2026-10-20 UTC with the existing qualified harness, with child executable
race building unset:

```text
go test -race -count=1 -timeout=4m -v ./acceptance -ginkgo.focus='Go source qualification|Published release qualification workflow contract' -ginkgo.no-color -ginkgo.fail-on-empty
16 Passed, 0 Failed; package 82.574s

go test -race -count=1 -timeout=3m -v ./acceptance -ginkgo.focus='Go source qualification worker execution' -ginkgo.v -ginkgo.no-color -ginkgo.fail-on-empty
1 Passed, 0 Failed; package 24.392s (after shared-bootstrap reuse)

go test -overlay=/private/tmp/factory-pr133-priority-removed-control/overlay.json -race -count=1 -timeout=3m -v ./acceptance -ginkgo.focus='Go source qualification worker execution' -ginkgo.no-color -ginkgo.fail-on-empty
0 Passed, 1 Failed; package 21.910s (expected negative control)
priority oracle: light cannot precede medium completion
```

Native discovery also retained exactly the same 44 full handoff identities as the
failed hosted report, with no additions or omissions. These controls establish
selection and scheduling behavior. The complete real handoff run and final-head
hosted timing remain separate outcomes; no speedup, published-release pass or
minimum-OS qualification is inferred from the small scheduling proof.
