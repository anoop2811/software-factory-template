# Source cutovers move prerequisite boundaries

Replacing a shell command with an adapter can move the first use of a compiler
or module cache earlier in an unchanged CI job. Audit executable consumers and
provision prerequisites before the earliest source invocation. Installed binary
consumers have a different dependency boundary; do not add a compiler there.

Provenance: observed 2026-11-04 while preparing Decision 149. At immutable
`9b7ef1ff4f2990309e94af61a19839e94e56cebf`, `.github/workflows/ci.yml:37`
runs structural evaluation before Go setup at line 57 and module acquisition
at line 62. `internal/installation/orchestration.go:20` already routes the
installed path to its native owner. The design is
docs/adr/0134-go-source-structural-evaluation.md:37; this lesson is a consumer
audit reminder, not a separate runtime contract.
