# ADR-0061: Authenticate and stage runtime bundles

Status: accepted for implementation, 2026-09-07; Decision 56.

## Boundary

Add the developer-only Cobra command `factory-stage`. This supplies the safe
staging prerequisite for specs/001-go-runtime-conversion.md:316 and
specs/001-go-runtime-conversion.md:322. It does not activate an installation,
execute a candidate, change adapters, or replace/retire legacy assets.

```
factory-stage --archive FILE --version VERSION --revision FULL_SHA --target GOOS/GOARCH --output NEW_DIRECTORY --attestation FILE --trusted-root FILE [--gh EXECUTABLE]
factory-stage --archive FILE --version VERSION --revision FULL_SHA --target GOOS/GOARCH --output NEW_DIRECTORY --local
```

All identity operands are required. Use ADR-0060's literal version/full lowercase
commit/four-target rules. Release mode is the default and requires an explicit
attestation and independently provisioned trusted-root file. `--local` is an
explicit unauthenticated source-build mode; reject combinations with attestation,
trusted-root or an explicit verifier. There is no fallback between modes.
Malformed requests return 2; authentication, archive, identity, integrity and
I/O failures return 1 with stderr diagnostics and no success JSON.

## Trust before interpretation

Snapshot regular, non-symlink archive/attestation/root inputs into an owned 0700
temporary directory with private 0600 files. Preserve filesystem semantics in
explicit paths; reject file-type changes between inspection and opening. Bound
the compressed archive to 128 MiB and each verification input to 16 MiB.

In release mode, call the explicitly chosen trusted GitHub CLI (default `gh` on
PATH), with a 30-second verification deadline and bounded captured diagnostics.
Use only snapshots, and bind `--repo anoop2811/software-factory-template`,
`--signer-workflow anoop2811/software-factory-template/.github/workflows/runtime-release.yml`,
`--source-digest FULL_SHA`, `--source-ref refs/tags/VERSION`, `--bundle FILE`,
`--custom-trusted-root FILE`, and `--deny-self-hosted-runners`. Pin the GitHub
host in the verifier environment. This uses the documented offline verification
path; never fetch roots, attestations or archives from inside the stager.

Do not parse or extract the archive until verification succeeds. Hash and extract
the same private archive snapshot; later changes to caller inputs must not change
the staged bytes. Local mode skips the verifier and reports `local-source`.
The caller is responsible for obtaining the verifier and roots through a trusted
channel. This is delegated verification, not a new cryptographic implementation.

## Strict extraction and publication

Accept exactly the three regular archive entries emitted by ADR-0060 under
`VERSION/GOOS/GOARCH/`: `bin/factory-runtime`, `runtime.manifest`, `source.json`.
Reject duplicate/extra/missing entries, traversal, absolute/alternate paths,
symlinks, hard links, devices, directories and unexpected modes. Expected archive
modes are 0755 for the binary and 0644 for metadata. Limit the binary to 256 MiB,
each metadata file to 64 KiB, and the entire decompressed stream to 257 MiB.
Validate gzip checksum/trailer; reject concatenated gzip streams and nonzero
data after tar's end. Never execute extracted content.

Reuse the existing artifact verifier for manifest, target and binary SHA-256.
Additionally bind manifest version and source metadata revision/version/target
to the explicit request; require `kind: source-build` and `go_version: go1.27.1`.
Reject unknown, duplicate, missing or trailing JSON fields/documents.

After all checks, publish only to an exclusively created output directory.
The resulting tree is `OUTPUT/VERSION/GOOS/GOARCH/{bin/factory-runtime,runtime.manifest,source.json}`.
Directories are 0700, binary 0700, metadata 0600. Existing paths, including
symlinks with directory suffixes, must remain untouched. Use confined filesystem
handles and exclusive file creation; never merge into an existing output.
Remove only owned temporary staging on failure. Interrupted final publication
may leave an incomplete new output; report failure and preserve it for inspection.

Success emits one JSON object with `version`, `revision`, `target`, `root`
(absolute output directory), `sha256` (archive snapshot digest), and
`authentication` (`github-attestation` or `local-source`). This result describes
this staging operation, not durable authorization for later activation. Activation
must revalidate ownership, integrity and release trust under its own lock.

## Evidence and integration

Independent Ginkgo/Gomega subprocess tests must fail before implementation.
Exercise actual archive bytes, preservation/failure paths, source/type/mode/JSON
validation, malformed/truncated streams, strict identity, and verifier invocation
ordering/refusal using a controlled fake CLI. Fake success establishes delegation
and gating only; it does not establish real signature acceptance.

The four-target native workflow stages its own bundle with explicit `--local`
and executes that staged binary for existing conformance tests with Go/Python
absent from child PATH. Production staging does not execute it. Live GitHub OIDC
acceptance still awaits a trusted release; macOS 14 Intel qualification and the
manual adopter pilot remain open. Public runtime routing is unchanged.

Authoritative verifier flags and offline semantics checked 2026-09-07:
https://cli.github.com/manual/gh_attestation_verify and
https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/verify-attestations-offline.
