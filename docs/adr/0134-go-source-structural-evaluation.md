# ADR 0134: native source structural-evaluation entrypoint

Status: accepted design; implementation and qualification evidence are separate

Date: 2026-11-04 UTC

## Scope

Replace `scripts/harness-structural-eval.sh` with a thin source adapter to the
existing `factory eval structural` owner. Preserve the native contract in
docs/adr/0099-native-factory-orchestration.md:99 and its sanitized adapter-read
failure in docs/adr/0099-native-factory-orchestration.md:244. This is one R3
function boundary; no new harness, model call, dependency or release default.

## Source invocation

Use the established physical-owner Bash adapter. Resolve the source checkout
without changing the caller's working directory. Preserve literal arguments,
stdin, environment, output streams and status through `exec`. Directory aliases
and trailing-newline owner paths retain their identities. Reject final-file
symlinks; PATH and runtime overrides cannot select another owner. Missing or
failed owners fail explicitly without executing the old implementation.

The caller's project supplies harness wiring and the shared test-edit denial
gate. Continue probing all three supported adapters with their existing literal
payloads and explicit implementer/spec-writer roles. A private temporary config
arms the probe independently of caller configuration and is removed afterward.
Keep exact success messages and the distinction between missing adapter input
and missing wiring markers. Structural success proves deterministic adapter
wiring; live in-harness behavior remains a separate observed gate.

Validation hooks remain their existing shared owners. This change does not
claim to remove their dependencies or qualify a live Codex, Claude or OpenCode
session. Installed adapters already route to the native owner; source execution
retains the existing source compiler requirement.

## Consumers and CI

Keep the public script path and the actual `make eval` recipe. Exercise the
structural steps from Go, TypeScript, Java/Gradle and Java/Maven pack workflows
against a private native layout. Do not make product toolchains responsible for
compiling the factory runtime in installed projects.

Move the template selftest job's existing Go setup and locked module acquisition
before its first native source invocation. Preserve action versions, acquisition
guards, lockfile checks and job topology. This closes a prerequisite ordering
dependency, not the paused test-performance experiment.

## Qualification and custody

An independent test author first observes the legacy source script fail the
new owner-routing criteria. Use real processes and literal transport witnesses,
not marker-only assertions. Qualify one actual private source compilation, the
three real role-denial probes, refusal paths, Make and pack workflow consumers.
Reuse existing native structural acceptance for the detailed command semantics.
Preserve the primary checkout's operator-owned binary and unrelated files.

Existing immutable structural oracles remain test-only evidence; do not turn
them into runtime fallback paths. Discover and update the actual source-suite
census after adding criteria. Run affected native acceptance, configured quality
gates, independent review and hosted checks. Record commands and real results
separately from this design. The source row closes only after review and merge;
R3 completion and the 0.2.0 release gates remain separate obligations.
