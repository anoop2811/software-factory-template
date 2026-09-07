# Go runtime artifact verification candidate

This slice adds a private verifier for a future packaged Go runtime. It checks
an inert `FACTORY_RUNTIME_ARTIFACT_V1` manifest, the requested `GOOS/GOARCH`, a
relative binary path below an artifact root, file types/symlink boundaries and
the binary's SHA-256 digest. A successful private request prints version,
target, relative binary path and digest as tab-separated data.

```sh
FACTORY_BRIDGE_PROTOCOL=1 ./factory-go runtime verify runtime.manifest dist linux/amd64
```

The command is a candidate boundary, not an installer. It never executes the
artifact, downloads anything, changes the repository or selects a fallback.
The existing public dispatcher and all installed scripts remain unchanged.
Source builds are local inputs and are not authenticated official releases.

The acceptance coverage exercises a valid artifact, digest mismatch, path
traversal, target mismatch, symlink refusal and malformed private operands.
This does not complete G1: signed provenance, release packaging, four-target
artifact qualification, staging, activation, backup/recovery and cleanup remain
required before any adopter-facing runtime switch.

## Select an exact local version

The private selector expects a store arranged as
`STORE/VERSION/GOOS/GOARCH/runtime.manifest`. Each manifest names its binary
relative to that target directory. For example:

```sh
FACTORY_BRIDGE_PROTOCOL=1 ./factory-go runtime resolve ./local-artifacts v0.2.0 linux/amd64
```

Only that slot is inspected. Its version, target and SHA-256 must match; a
missing or invalid slot returns failure even when other versions are available.
Success prints version, target, binary path relative to the store and SHA-256
as four tab-separated fields. This command never executes the selected binary.
The version is a literal label; moving aliases such as `latest` are rejected.
It does not prove publisher authentication or bind a release label to a source
revision. A later activation must perform its own locked validation.

This is the FR-020 local selection prerequisite, described in
[ADR-0059](../adr/0059-deterministic-runtime-selection.md). Binary distribution
now follows the approved platform and authentication decisions in Q2/Q3;
qualification and lifecycle exits still remain.

## Build an exact committed source bundle

The developer-only Cobra packager requires the pinned Go 1.27.1 toolchain, Git,
and pre-provisioned module dependencies. Adopters do not need these tools.

```sh
go mod download
revision=$(git rev-parse HEAD)
go run ./cmd/factory-package --source . --revision "$revision" \
  --target darwin/arm64 --output /tmp/factory-source-bundle
```

Choose `linux/amd64`, `linux/arm64`, `darwin/amd64` or `darwin/arm64`.
The output directory must not exist. Working-tree changes are ignored, so commit
the desired source first. Builds use offline module lookup and never fall back
to another compiler or revision. Filesystem module replacements are rejected.
`--go` selects an explicit matching compiler (the default is `go` on PATH);
`--version` selects a literal label (default `local-REVISION`).

The bundle contains an exact-version local store, `factory-runtime.tar.gz` and
`SHA256SUMS`. Each slot contains a binary, manifest and `source.json` describing
the revision, compiler and target. Archive contents omit the `store/` prefix.
The archive normalizes order, timestamps and ownership for repeatability.
An interrupted publication can leave an incomplete newly created output folder;
inspect it and choose a fresh output path before retrying. Existing files are
never overwritten or recursively removed by the packager.

This is a candidate binary. Only the private Go bridge operations are standalone;
the public dispatcher still delegates unmigrated commands to installed scripts.
Do not replace an adopter's runtime or retire legacy files using this bundle.

## Native CI and release authentication

The native bundle workflow builds and exercises the actual packaged executable
with Go/Python absent from its PATH on all four targets:

| Target | Hosted qualification runner | Minimum-OS evidence |
| --- | --- | --- |
| linux/amd64 | Ubuntu 24.04 | CI execution required |
| linux/arm64 | Ubuntu 24.04 ARM | CI execution required |
| darwin/amd64 | macOS 15 Intel | macOS 14 Intel pilot still required |
| darwin/arm64 | macOS 14 ARM | CI execution required |

The approved support scope is Ubuntu 24.04+ and macOS 14+. A newer runner does
not establish support for an older minimum. The Intel macOS 14 gap must close
before claiming the whole platform set is qualified or enabling distribution
as the default runtime.

After a maintainer publishes a release, `runtime-release.yml` builds its exact
tagged revision, runs the native checks, attests the four archives with GitHub's
hosted identity, verifies source/ref/workflow bindings, and attaches assets to
that existing release. It does not create releases or overwrite existing assets.
PR workflows have no signing permissions. No hosted signing result is claimed
until this release workflow has actually run successfully.

For offline verification, provision GitHub CLI and trust roots independently
on a trusted online machine (`gh attestation trusted-root > trusted-root.jsonl`).
Transfer those roots through your trusted channel, separately from the release
archive and `runtime-attestations.jsonl`. Set the expected full source commit
and release tag from a trusted release record, then verify before extraction:

```sh
gh attestation verify "$archive" --repo anoop2811/software-factory-template \
  --signer-workflow anoop2811/software-factory-template/.github/workflows/runtime-release.yml \
  --source-digest "$expected_commit" --source-ref "refs/tags/$expected_tag" \
  --bundle runtime-attestations.jsonl --custom-trusted-root trusted-root.jsonl \
  --deny-self-hosted-runners
```

Checksums alone establish integrity, not publisher identity. Offline roots need
independent refresh; bundled roots are not automatically trusted. Authenticated
installer integration, locked activation, backups, recovery and retirement remain
future migration slices. See [ADR-0060](../adr/0060-runtime-source-bundles.md).
