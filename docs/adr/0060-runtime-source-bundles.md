# ADR-0060: Runtime source bundles and attested delivery

Status: accepted for implementation, 2026-09-07. Anoop approved Q2/Q3 in chat.

## Contract

Add a developer-only Cobra packager, separate from the installed dispatcher:

```
go run ./cmd/factory-package --source REPO --revision FULL_SHA --target GOOS/GOARCH --output NEW_DIRECTORY [--version LABEL] [--go COMPILER]
```

The source revision is a complete lowercase 40-character Git commit identity,
not a branch/tag expression. Build only committed `go.mod`, `go.sum`,
`cmd/factory` and `internal` from that revision via a Git archive into an owned
temporary directory; working-tree edits and untracked files are not build input.
Reject archive symlinks and unsafe paths. Never execute the resulting binary.

Default version is `local-REVISION`. An explicit version must use ADR-0059's
literal version syntax and is still only a label, not proof of authenticity.
Permit exactly linux/amd64, linux/arm64, darwin/amd64 and darwin/arm64. The
approved qualification scope is Ubuntu 24.04+ and macOS 14+. No Windows claim.

Use the existing Go 1.27.1 compiler pin, CGO_ENABLED=0, `-trimpath`,
`-buildvcs=false`, disabled workspace/ambient GOFLAGS/experiments, baseline
architecture settings and offline module lookup (GOPROXY=off, GOSUMDB=off,
GOTOOLCHAIN=local). The default compiler is `go` on the developer's PATH;
`--go` may select another explicit compiler, but its version must match the pin.
Dependencies must be provisioned before this explicit developer build; reject
filesystem module replacements. Failures include diagnostics and never retry.

OUTPUT must not already exist (including a symlink). Preserve existing paths.
Stage under an owned temporary directory and remove that staging on failure.
Publish only to a newly created output directory; never merge/overwrite another
bundle. Success emits one JSON object containing `version`, `revision`,
`target`, `archive` (absolute archive path), and `sha256` (archive digest).
Usage/invalid operands return 2; source/compiler/build/output failures return 1.

OUTPUT contains `store/VERSION/GOOS/GOARCH/bin/factory-runtime`, the existing
`runtime.manifest`, and `source.json` in the same slot. `source.json` records
`revision`, `version`, `target`, `go_version` and `kind: "source-build"`.
Also emit `factory-runtime.tar.gz` containing exactly those three slot files
(without the `store/` prefix), and `SHA256SUMS` naming the archive. Normalize
tar/gzip timestamps, owners and file ordering; executable mode is 0755, metadata
0644. Identical source/compiler/target/version inputs yield identical archive
bytes. Local builds remain unauthenticated until independently attested.

## Delivery and qualification

A four-target native PR workflow builds bundles and runs packaged checks using
the actual bundled executable. Public runtime migration remains disabled; this
artifact contains the Go candidate only, not every legacy script it delegates to.
No new Go or Python prerequisite is added to adopter repositories.

A separate release workflow builds the selected release tag revision, runs
the packaged checks, then attests the archive using GitHub's hosted OIDC
identity. Only release events from this repository can obtain signing/write
permissions. Save the attestation bundle beside the archive and checksums.
Offline verification uses a separately provisioned GitHub CLI and trusted-root
file, binding repository, exact workflow, source revision and release ref before
extracting or executing any artifact. Bundled roots are never automatic trust.
No release is created/published by implementing this PR.

Native runner labels and actual minimum-OS evidence must be documented honestly;
a newer OS runner does not prove the minimum older OS. All four target builds
are required; unsupported runner availability must not silently reduce the matrix.

Independent Ginkgo/Gomega CLI tests run RED before build implementation, then
cover archive contents/digests, repeatability, committed-source isolation,
native packaged execution with no Go/Python on PATH, malformed inputs, missing
revision, compiler/build failure and existing-output preservation. Release OIDC
signing is verified only when its trusted release workflow actually runs.

This advances FR-019/020/022 (specs/001-go-runtime-conversion.md:322), not default
cutover. Adopter activation, authenticated installer integration, backups and
legacy retirement remain lifecycle work; no legacy assets are replaced here.

## Authoritative sources checked 2026-09-07

- Go 1.27.1, released 2026-09-01: https://go.dev/doc/devel/release
- actions/attest v4.2.2, released 2026-08-04: https://github.com/actions/attest/releases/tag/v4.2.2
- actions/upload-artifact v7.0.1, released 2026-04-10: https://github.com/actions/upload-artifact/releases/tag/v7.0.1
- actions/download-artifact v8.0.1, released 2026-03-11: https://github.com/actions/download-artifact/releases/tag/v8.0.1
- Hosted runner labels: https://docs.github.com/en/actions/reference/runners/github-hosted-runners
- Offline verification and independent roots: https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/verify-attestations-offline
- Source/workflow/ref verification flags: https://cli.github.com/manual/gh_attestation_verify
