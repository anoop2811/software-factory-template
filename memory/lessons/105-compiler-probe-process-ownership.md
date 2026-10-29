# Compiler probes need the same process ownership as builds

Observed 2026-10-29 via a private public `init --local --source` rehearsal and an
independent Ginkgo compiler/descendant readiness criterion: context cancellation
killed the direct compiler while a descendant's inherited stdout kept capture
blocked beyond six seconds. Even a version probe can create this boundary.

Reuse the existing supervised executor for compiler probes, module inspection and
builds. A leader's exit does not prove group cleanup, and uncertain ownership
must retain the private workspace with a diagnostic. Canon and qualification are
in [ADR 0110](../../docs/adr/0110-local-compiler-process-ownership.md).

Provenance: the real pre-correction handshake failure and the exact-source
negative control are summarized in ADR 0110's local qualification; twelve
current criteria passed, while the old implementation retained one healthy
control and failed the other eleven behavior checks.
