# Native calls must preserve subprocess presentation boundaries

Replacing a captured subprocess with a direct function can expose a terminal
that the subprocess never saw. During native review-lane integration, init had
already accepted the default review model; the direct lane call saw terminal
stdout and prompted again. Keep interactive presentation explicit while retaining
the real output descriptor needed for cancellable writes.

Preflight mutation prerequisites before irreversible removal. The native disable
test showed a managed workflow removed before a missing config was discovered.
Also distinguish config rollback from artifact absence: rolling lane-off does not
prove a previously installed workflow disappeared.

Provenance: observed 2026-09-30 via compiled acceptance selection
`preserves the managed workflow when disable|Native Go init interactive review choice.*frontier default`,
which reported `0 Passed | 3 Failed` in `26.900s`. Internal publication tests
reported `1 Passed | 2 Failed` on the false absence diagnostic. The contract and
corrections live in docs/adr/0089-go-native-review-lane.md:115.
