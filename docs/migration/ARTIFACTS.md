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
still awaits the platform qualification and authentication decisions in Q2/Q3.
