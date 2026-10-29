# Qualify a published Go release candidate

The contract is [ADR 0107](../adr/0107-published-release-qualification.md).
This repository-only lane exercises actual published assets and their GitHub
attestations. Regular users continue to use the single `factory` CLI.
Adding the lane does not establish that a release passed it. The
[replacement-release checklist](GO_RELEASE_READINESS.md) remains authoritative.

Decision 118 records the 2026-10-28 operator approval of Ubuntu 24.04+ on both
architectures, macOS 15+ on Intel and macOS 14+ on Apple Silicon. Historical
candidate tables below retain their original policy and outcomes. Finish the
remaining cutover implementation, cleanup and local supported-project pilot
before one RC4; actual published authentication and migration qualification run
after its reviewed source is merged and the candidate is published.

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

## Second published candidate result

On 2026-10-26 UTC, [v0.2.0-rc.2](https://github.com/anoop2811/software-factory-template/releases/tag/v0.2.0-rc.2)
was published at `f6c179abba9e1062452f6cd6a51ba693fbcfa0f8`, the reviewed merge of
PR #137. Its tree matches head `b600f249e39972f70b7e6f164b7c757f504211c1`, which
passed all eighteen checks. The merged source Go matrix also passed all six jobs.
[Release run 40252592120](https://github.com/anoop2811/software-factory-template/actions/runs/40252592120)
passed four builds, authentication of all twelve subjects, upload of fourteen
assets and the release guard. Each evaluator independently authenticated its
three native subjects. RC1's extensionless-bundle refusal no longer occurs.

| Target | Observed OS | Published scenarios |
|---|---|---|
| Linux amd64 | Ubuntu 24.04.5 LTS | 1 passed, 4 failed |
| Linux arm64 | Ubuntu 24.04.5 LTS | 0 passed, 5 failed |
| macOS amd64 | 15.7.9 | 1 passed, 4 failed |
| macOS arm64 | 14.8.9 | 1 passed, 4 failed |

The passing scenario on three targets is fresh authenticated Go initialization,
including installed doctor. Failures remain separately classified:

- Both upgrade/rollback criteria stop at the evaluator's conflict negative
  control: status 2 returns a JSON proposal, but the
  [RC2 assertion at line 321](https://github.com/anoop2811/software-factory-template/blob/f6c179abba9e1062452f6cd6a51ba693fbcfa0f8/acceptance/published_release_test.go#L321)
  incorrectly requires stderr. That
  assertion does not establish that the clean upgrade or rollback body failed.
- On three targets, both interruption criteria reach successful first recovery
  and installed checks, then repeated recovery preview refuses with
  `terminal pending observation differs from current marker` at
  [RC2 line 508](https://github.com/anoop2811/software-factory-template/blob/f6c179abba9e1062452f6cd6a51ba693fbcfa0f8/acceptance/published_release_test.go#L508). The
  committed journal retains its historical pending identity after cleanup removed
  the marker; completed recovery needs a separately requalified no-op path.
- Linux arm64's fresh initialization and later installed checks observe a shared
  [forbidden-tool flag](https://github.com/anoop2811/software-factory-template/blob/f6c179abba9e1062452f6cd6a51ba693fbcfa0f8/acceptance/published_release_test.go#L227).
  The original artifact lacks its contents, so it does not
  identify which tool/caller wrote it or prove that each later command repeated
  the attempt. Optional npm native-addon setup is a concrete candidate requiring
  controlled execution and improved diagnostic evidence. Keep the control intact.

The retained `published-release-TARGET-40252592120-1` artifacts include release
identity, authenticated subjects, observed OS, Ginkgo JSON/JUnit and actual stopped
interruption journals. Counts above include only the five scenario criteria, not
Ginkgo suite hooks. The twenty criteria total three passes and seventeen failures.
Decision 113 records the correction contract before implementation. RC2 notes mark
it unqualified for migration; source, tag and assets remain unchanged. v0.1.6 stays
latest stable. No platform, adopter-pilot or stable-release gate closes here.

## Third published candidate result

On 2026-10-26 UTC, [v0.2.0-rc.3](https://github.com/anoop2811/software-factory-template/releases/tag/v0.2.0-rc.3)
was published at `95486ecdd3868a319a0e6362ee99cafd8f154200`, the reviewed merge of
PR #138. Its tree matches reviewed head `2c9768e1d90f7a7112498e2b9b13162ba0262ead`,
which passed all eighteen checks. The adversarial review completed fourteen files
and thirty-six fragments; independent reviewers checked and answered its claims.
[Release run 40275363119](https://github.com/anoop2811/software-factory-template/actions/runs/40275363119)
passed all ten jobs: four native builds, attestation/authentication and upload,
the guard and four actual published qualification jobs. Fourteen assets are
published; all twelve authenticated subject names/digests match the checksum
records. Candidate, harness and workflow source identities all match the expected
commit.

| Target | Observed OS | Actual live criteria | Minimum OS established |
|---|---|---|---|
| Linux amd64 | Ubuntu 24.04.5 LTS | 5 passed, 0 failed | Yes |
| Linux arm64 | Ubuntu 24.04.5 LTS | 5 passed, 0 failed | Yes |
| macOS amd64 | 15.7.9 | 5 passed, 0 failed | No: macOS 14 Intel remains open |
| macOS arm64 | 14.8.9 | 5 passed, 0 failed | Yes |

The twenty passes count only executed live `It` criteria, excluding setup/cleanup
hooks and focused-out source cases. No live criterion was skipped. Every target
passed fresh initialization; genuine v0.1.6 and immutable Bash-baseline
upgrade/rollback; and witnessed interruption/recovery for both predecessors.
Preservation, ignored inert recovery, forbidden-tool controls and fresh completed
recovery replay assertions ran in those bodies. Eight retained applying journals
supply the actual interrupted-state evidence. No paid model was called.

The original `published-release-TARGET-40275363119-1` artifacts retain exact
release/harness identity, observed OS, three independently authenticated native
subjects per target, Ginkgo JSON/JUnit and stopped transaction evidence.
The invoked live command was:

```text
go test -tags=publishedrelease -race -count=1 -timeout=25m -v ./acceptance -ginkgo.focus='^Go factory command acceptance Published release qualification live ' -ginkgo.fail-on-empty -ginkgo.no-color -ginkgo.json-report=EVIDENCE_DIR/ginkgo.json -ginkgo.junit-report=EVIDENCE_DIR/junit.xml
```

All four original workflow logs report:

```text
SUCCESS! -- 5 Passed | 0 Failed | 0 Pending | 3009 Skipped
```

The ordinary source suite has 3,008 criteria. The publishedrelease build tag adds
six: five live scenarios and one evaluator control, for 3,014 registered criteria.
Selecting the five live scenarios leaves 3,009 focused-out criteria: the 3,008
ordinary criteria plus the evaluator control (which also runs inside live setup).
Those skipped registrations are not skipped live migration scenarios.
Independently counting only the five executed live criteria per report gives
twenty passed, zero failed and zero skipped live scenarios overall.

`EVIDENCE_DIR` above abbreviates each runner's actual absolute temporary artifact
directory. The source remains immutable; release notes record these outcomes.
The separate merged-source run 40275252277 has a repeated healthy-control Git
initialization timeout in the source handoff fixture. Decision 114 keeps that
source-CI investigation separate from the successful published result; it does
not attribute the host delay without evidence.

RC3 establishes these published outcomes at the observed operating systems.
It does not establish macOS 14 Intel compatibility, mandatory operator pilot
acceptance, default Go selection, final classified retirement/audit or a stable
release. v0.1.6 remains latest stable; RC1 and RC2 stay unchanged and marked
unqualified. See the [remaining release gates](GO_RELEASE_READINESS.md).

## Local correction evidence

Decision 110 preserves the original bundle format suffix in the private snapshot
used by the generic stager. Independent public regressions first observed two
failures for JSON/JSONL and one existing unsupported-format refusal pass. After
the correction, the same three criteria passed. They exercise a compiled consumer
with GitHub-compatible suffix dispatch, private permissions, unchanged bytes after
caller-input mutation, staged payload identity and refusal without publication.
The source census is now 2,999 ordinary criteria; no previous criterion was removed.

Observed 2026-10-24 with Go 1.27.3 (released 2026-10-19;
[official Go release history](https://go.dev/doc/devel/release)). The linter below
is the repository's existing v2.14.0 CI pin, not a claim of the latest tool version:

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

## Correction-PR CI follow-up

Head `33507ff51ff974e173bc6941714180a8cebecd15` passed Linux base,
macOS installation, both release-handoff jobs and the four native bundle jobs.
[Run 40105875345](https://github.com/anoop2811/software-factory-template/actions/runs/40105875345)
also retained two distinct failures:

- Linux installation: the existing committed-cleanup interruption criterion sent
  SIGSTOP and read a pending witness without first observing that the process had
  stopped. After SIGKILL, that marker was absent. The observer follow-up requires
  actual native stopped state before reading durable crash evidence; it preserves
  signal-exit, pending, recovery and bounded-attempt assertions. The live published
  recovery fixture uses the same observer before accepting its partial journal.
- macOS base: an unchanged archive-subject criterion failed its healthy initialization
  control because the real isolated `git init` proof exceeded its existing
  five-second limit (`elapsed_seconds=5.023`, exit confirmed). This is before the
  wrong-subject scenario. The log does not establish the source of the delay;
  hosted contention is only a hypothesis. Preserve the production deadline and
  assertion, then inspect the next head's native CI outcome.

An independent compiler-overlay probe withheld only the first actual SIGSTOP,
waited for healthy `Cmd.Wait` completion, requeued that result and returned a
successful signal-call result. The old observer tried SIGKILL against the exited
child: `0 Passed | 1 Failed` in 120.186 seconds. With actual-stop observation, the
same injected acknowledgement race produced `4 Passed | 0 Failed` in 214.154
seconds. Its retained Ginkgo output records an unwitnessed healthy completion on
`terminal-cleanup-1`, then an actual committed/pending SIGSTOP/SIGKILL witness on
`terminal-cleanup-2`, plus the separate real applying-row witness. It does not
credit the omitted stop as a crash or introduce a production fault flag.

The final ordinary recovery run on Darwin arm64, without the compiler overlay,
passed all four selected criteria in 195.659 seconds. It retained real committed
and applying-row stopped witnesses, exact SIGKILL checks, pending admission and
forward/reverse recovery. Linux amd64 cross-compilation exited 0; native Linux
behavior remains the CI matrix's evidence. The tagged live lane compiled and
selected its five criteria in dry-run mode only; no published test body ran.

```text
env GORACE=atexit_sleep_ms=0 GOFLAGS= go test -race -count=1 -timeout=5m -v ./acceptance -ginkgo.focus='Whole installation upgrade semantic consumer (recovers actual committed pending cleanup without replaying assets or making another backup|preserves a foreign equal-byte created leaf after an actual applied-row interruption|retains one pending installation after native interruption and recovers with fresh consent)' -ginkgo.no-color -ginkgo.fail-on-empty
Ran 4 of 2999 Specs in 195.659 seconds
SUCCESS! -- 4 Passed | 0 Failed | 0 Pending | 2995 Skipped
```

The unchanged archive-subject criterion also passed a focused native macOS run:

```text
go test -race -count=1 -timeout=6m -v ./acceptance -ginkgo.focus='Authenticated release candidate archive subject refuses a different verified archive subject even when raw bootstrap authentication succeeds' -ginkgo.no-color -ginkgo.fail-on-empty
Ran 1 of 2999 Specs in 23.962 seconds
SUCCESS! -- 1 Passed | 0 Failed | 0 Pending | 2998 Skipped
```

That local healthy control does not determine the previous hosted delay's cause
or replace the next head's full native CI run.

These failures remain separate from the observed attestation-format defect.
A passing source rerun cannot qualify the unchanged failed RC1.

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

The current approved floors are Ubuntu 24.04+ on both architectures, macOS 15+
on Intel and macOS 14+ on Apple Silicon (Decision 118 and Q2). Record the actual
native OS family and architecture. A pass on a newer supported OS does not
establish execution at the approved minimum. RC1-RC3 retain their original
macOS 14 Intel gap; do not rewrite historical reports under the new policy.
The announced retirement of the hosted macOS 14 runner requires an explicit
qualification plan for future Apple Silicon minimum-OS runs; this lane neither
silently raises that floor nor provisions paid runners.

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
