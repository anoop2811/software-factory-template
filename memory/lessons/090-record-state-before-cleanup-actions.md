# Record state before cleanup actions

A final `reaped=true` result does not establish whether reap happened before a
failed signal or during subsequent cleanup. Capture the state at the decision
point as well as the final state; otherwise a precise errno can still leave the
causal ordering ambiguous. The native diagnostic records `reaped_before_signal`
at `internal/native/execute.go:299` and formats only bounded safe evidence.

Darwin group EPERM is not proof of an active permission denial or of absence.
Apple's group-signal implementation excludes zombie members and returns EPERM
when none are eligible. The canonical qualification and narrow terminal-proof
refinement are in `docs/adr/0103-darwin-natural-exit-reconciliation.md:31`.

Provenance: observed 2026-10-18 via PR #129's retained macOS report from Actions
run 39159920359, which reported EPERM with final reap/drain booleans; source fetched
2026-10-18 UTC:
https://github.com/apple-oss-distributions/xnu/blob/f6217f891ac0bb64f3d375211650a4c1ff8ca1ea/bsd/kern/kern_sig.c#L1675
