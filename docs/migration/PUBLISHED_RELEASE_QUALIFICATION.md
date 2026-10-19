# Qualify a published Go release candidate

The contract is [ADR 0107](../adr/0107-published-release-qualification.md).
This repository-only lane exercises actual published assets and their GitHub
attestations. Regular users continue to use the single `factory` CLI.
Adding the lane does not establish that a release passed it. The
[replacement-release checklist](GO_RELEASE_READINESS.md) remains authoritative.

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
execution. The independent real GitHub verifier authenticates the exact native
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

Independent correctness, security and test review plus finding verification found
no surviving source finding. The five real published-release scenarios remain
NOT RUN; neither this evidence nor a source-CI pass closes that release gate.
