# Reverse preflight needs durable custody

A completed installation already owns durable journal evidence. Reusing that
journal for rollback health checks before a recoverable pending transition exists
can strand recovery when a prerequisite is missing or the owner exits. A cleanup
defer alone cannot cover process termination between durable writes.

Require a recoverable preparation boundary before replacing committed evidence,
and test a failed reverse health check followed by supported retry/recovery.
Preserve uncertain-child ownership; never synthesize a marker in a pilot to make
recovery pass.

Provenance: observed 2026-11-17 through actual published RC4 supported upgrade,
rollback with unavailable legacy Python, fresh rollback preview and reverse
recovery preview. The diagnostics and unchanged-application scope are recorded
in docs/migration/RC4_ADOPTER_PILOT.md. Canonical correction contract:
docs/DECISION_LOG.md:8986. This lesson is a pointer, not a second protocol spec.
