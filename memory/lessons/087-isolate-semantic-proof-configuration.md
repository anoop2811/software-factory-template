# Isolate configuration as well as environment in semantic proofs

Observed 2026-10-18 via the independent real-Git regression in
internal/initcmd/ignore_config_test.go:19: identical canceled private ignore rules
were rejected with a clean home but accepted when a global excludes file supplied
the missing rule. Removing inherited `GIT_*` values alone did not isolate the
proof from configuration discovered through `HOME`.

The canonical requirement is docs/adr/0101-go-release-handoff.md:515. Explicitly
disable global and system configuration for this verifier after sanitizing its
environment. Keep local repository evidence and user configuration intact. A
helper-level counterexample proves a proof boundary defect; it does not establish
a public data exposure when an earlier layer always adds the protective rule.
