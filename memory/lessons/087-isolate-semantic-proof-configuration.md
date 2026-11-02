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

Follow-up: observed 2026-11-01 via an isolated native-sync Ginkgo run with
exported host model settings. The fixture filtered those settings, but its
process helper merged omitted parent keys back into the child. Generated output
contained `ambient-model` instead of `caller-model` (2 passed, 1 failed).
Passing the exact selected environment through the shared bounded runner then
passed all three sync criteria and two existing overlay-consumer criteria under
the same poison. Canon is docs/adr/0128-go-source-harness-sync-entrypoints.md:92;
[Copilot's finding](https://github.com/anoop2811/software-factory-template/pull/159#discussion_r4437832965)
identified the second merge. Sanitization must hold at the final execution
boundary, while existing callers can retain their intended overlay behavior.
