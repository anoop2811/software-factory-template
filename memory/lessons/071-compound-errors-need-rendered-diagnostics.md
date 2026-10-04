# Compound errors need rendered diagnostics

Changing Python `OSError.args` can leave its displayed message unchanged because
the error renders its errno and strerror fields. Preserving the primary exception
type and chaining a cleanup cause therefore does not establish that a caller
which prints only `str(error)` reports both failures.

Observed 2026-10-04 UTC via the actual budget controller's committed terminal
write followed by an injected I/O error and a reported guard-close failure:
the direct error preserved OSError and errno 5 but omitted the cleanup diagnostic.
The failing assertion is acceptance/runtime_transition_python_test.go:308;
the full RED log is
`/private/tmp/factory-runtime-transition-oserror-compound-red.log` (4 passed,
1 failed). The marker remained present, so this was a reporting failure rather
than lost exclusion evidence.

Observe the direct diagnostic and structured primary evidence at the consuming
boundary when qualifying compound errors. The requirement remains in
[ADR 0093](../../docs/adr/0093-runtime-transition-guard.md); this lesson adds no
exception policy or recovery authority.
