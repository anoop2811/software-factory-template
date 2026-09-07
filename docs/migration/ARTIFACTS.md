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
