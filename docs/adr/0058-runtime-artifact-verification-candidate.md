# ADR-0058: Go runtime artifact verification candidate

Status: accepted for candidate implementation, 2026-09-07 UTC.

Continue the highest-priority Go conversion with a private G1 packaging slice.
The candidate verifies a release-matched, platform-specific runtime artifact
before any future adapter or installer executes it. It does not install,
download, activate, replace, remove or fall back to another runtime.

The inert manifest format is:

```
FACTORY_RUNTIME_ARTIFACT_V1
version<TAB>release identity
target<TAB>GOOS/GOARCH
binary<TAB>relative path below the supplied artifact root
sha256<TAB>64 lowercase hexadecimal characters
END
```

Each field appears exactly once. Verification requires a regular, non-symlink
manifest and binary, rejects absolute or traversal paths, requires the target
to equal the caller-selected target, and compares the computed SHA-256 digest
before returning success. The private Cobra request is
`runtime verify MANIFEST ARTIFACT_ROOT TARGET`; status 0 emits deterministic
metadata, status 1 reports an I/O or integrity failure, and status 2 rejects
malformed operands or an unsupported private request.

The verifier uses only the standard library and context cancellation. It never
executes the artifact, follows a symlink, mutates the repository, or changes
the existing public `factory` dispatcher. Source builds remain local candidate
inputs and are not authenticated official releases. Installation, signed
provenance, staging, backup/recovery, activation and cleanup remain later G1/G4
work and this slice must not be described as packaged delivery.

Evidence target: AC 5.2, AC 5.4, AC 6.4 and FR-019/020/022 of
`specs/001-go-runtime-conversion.md`.
