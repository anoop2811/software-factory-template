# ADR-0059: Deterministic local runtime selection

Status: accepted for candidate implementation, 2026-09-07.

## Decision

Implement the FR-020 prerequisite independently of release publication and
platform qualification. The private Cobra request is:

```
FACTORY_BRIDGE_PROTOCOL=1 factory-go runtime resolve STORE VERSION TARGET
```

Resolve exactly `STORE/VERSION/GOOS/GOARCH/runtime.manifest`, with binary paths
relative to that target directory. Require the manifest version and target to
equal the requested values and run the existing artifact integrity verifier.
Never enumerate versions, search PATH, consult environment-selected alternate
stores, execute the candidate, compile, download, retry another version or
fall back to shell. A missing selected slot fails even if another valid slot
exists. Caller operands are explicit; the host platform never overrides TARGET.

VERSION is one ASCII path component, 1-128 characters, beginning with a letter
or digit, then letters, digits, dots, underscores, plus or hyphens. Reject the
moving aliases `latest`, `current`, `main` and `HEAD` (case-insensitively).
This syntax accommodates release labels and explicit `local-REVISION` labels;
it does not authenticate a release label or prove source provenance.
TARGET uses the existing verifier's GOOS/GOARCH syntax. All store-to-slot
directory components must be real directories, not symlinks. Store suffixes
are not a bypass: strip terminal separators and `/.` before
checking STORE, preserving the meaning of interior `..` components.
STORE is then resolved to its physical directory before joining slot components.
The shared verifier rejects raw binary `..` path components before cleaning,
so a hidden symlink in a canceled path prefix cannot bypass binary checks.
Malformed operands return 2; missing inputs, symlinked/non-directory slots,
version mismatch and integrity failures return 1. Preserve verifier status
classes for malformed binary paths.

Success emits the existing four tab-separated fields: version, target, binary
path relative to STORE (including VERSION/GOOS/GOARCH), SHA-256, then newline.
Preserve the manifest's validated relative binary spelling when prefixing its
slot, including harmless `.` components.
Store paths containing spaces are valid. Output failures return 1. A returned
path is an inspection result, not authority for later execution: future
activation must authenticate and revalidate it under its lifecycle lock.

## Boundaries and evidence

This local selector neither publishes artifacts nor activates an adopter
runtime. Minimum OS qualification (Q2), publisher authentication (Q3), packaged
delivery, source revision binding, installation, backup/recovery and cleanup
remain prerequisites for adoption. No legacy assets are replaced by this slice.
The public dispatcher and Codex, Claude Code and OpenCode adapters retain their
current contracts. No language pack or model settings change.

An independent evaluator owns Ginkgo/Gomega subprocess tests: exact selection
among multiple versions, missing/corrupt selection without fallback, version
and target mismatch, symlink directories, malformed arguments and untouched
executable sentinels. Run these RED before implementing the command.
See specs/001-go-runtime-conversion.md:240 and
specs/001-go-runtime-conversion.md:323.
