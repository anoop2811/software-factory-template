# Successful helper exit is not group completion

A local help or Git helper can exit successfully after starting a same-group
descendant that closes its output streams. A successful leader status, finished
pipe capture and terminal controller record therefore do not establish owned
group completion. A transition marker must not disappear on those facts alone.

Observed 2026-10-04 UTC via independent actual legacy-budget and compiled/legacy
manual-loop regressions: status 0, completed accounting/checkpoint evidence and
zero activity markers coexisted with a live helper descendant after a PID/ready
handshake. The failing controls are acceptance/runtime_transition_test.go:115
and acceptance/runtime_transition_test.go:163; the original full logs are
`/private/tmp/factory-runtime-transition-help-descendant-red.log` and
`/private/tmp/factory-runtime-transition-snapshot-descendant-red.log`.

Reuse owned supervision for successful probes too, rather than adding another
leader-only wait. The controlling requirements and qualification remain in
[ADR 0093](../../docs/adr/0093-runtime-transition-guard.md) and
[runtime transition evidence](../../docs/migration/RUNTIME_TRANSITIONS.md).
This lesson grants no migration, recovery or pruning authority.
