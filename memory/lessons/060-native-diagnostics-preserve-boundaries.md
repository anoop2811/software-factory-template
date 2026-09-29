# Native diagnostics preserve process and filesystem boundaries

Moving report orchestration into Go does not make stream ordering or temporary
cleanup incidental. The first doctor report parity run exposed stderr being
appended after stdout, which changed failed-proof evidence. Use the existing
supervisor's combined-stream mode when the shell redirected both streams together.

A scratch copy also inherits read-only directory modes. Removing the copy can
then fail even though its creator owns it. Cleanup must restore removal access
within the owned root, preserve unknown replacement identities, and report errors
instead of calling the diagnostic healthy while leaving private copies behind.

Provenance: observed during native doctor qualification on 2026-09-29. Frozen
proof parity in acceptance/native_doctor_test.go:492 first failed, and the final
instrumented doctor matrix passed in 130.941s. The read-only scratch regression
at acceptance/native_doctor_test.go:294 passed after correction. An unsafe manual
substitution probe was discarded separately; it is not evidence for cleanup
identity protection. Later guarded acceptance at
acceptance/native_doctor_test.go:197 passed with CLI and outer race checking in
4.762s; that is post-implementation GREEN evidence only. ADR-0086 remains the source of truth for diagnostic behavior.
