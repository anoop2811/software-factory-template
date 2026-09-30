# Native Go advisory review-lane management

Decision 88 and [ADR-0089](../adr/0089-go-native-review-lane.md) define the native
`review-lane` service. Status, enable, disable, pending and secret-name no longer
require the legacy lane script or config/color shell libraries. Init and doctor
reuse the same service with explicit project roots.

The command preserves ordinary output and provider defaults. It reads secret
names through one bounded `gh secret list` call when needed; it never uploads
secrets or calls a model. Hosted review behavior and provider settings are
unchanged. Invalid GitHub secret names are rejected before rendering workflow
expressions. Model answers must fit the existing flat config representation.

Enabling stages a complete workflow, records settings and then publishes the
workflow atomically. Exact first-line ownership is required before replacement
or removal. Unsafe workflow paths are refused. Failed publication attempts to
record lane-off under an independent deadline; the diagnostic does not claim
that a previously installed workflow disappeared. This is not a two-file atomic
transaction, and the prior workflow can remain after a failed replacement.

Disabling first checks configuration writability, preventing a missing/read-only
config from causing removal followed by failure. Explicit config overrides keep
the existing setter's path semantics and trusted-directory contract. Generated
workflows receive no recovery backups; identified staging files are cleaned.

The native init integration explicitly suppresses a second model prompt and lane
color, preserving the old captured-child presentation behavior. It retains the
actual output descriptor for cancellation. Terminal input and staged publication
are shared with existing init and metrics code where their contracts match.

## Qualification

Core tests first reported `0 Passed | 3 Failed`, and nine frozen ordinary parity
cases also failed on the old dispatch route. The reference is commit
`315e4f90250260ca2933ade74063cdbd532cc5ad`. Separate init/doctor integration cases
failed before native wiring. Further tests exposed misleading rollback wording,
partial disable with an unusable config, and a repeated blank-model init prompt.

Compiled-process cases exercise controlling-terminal prompts, signal cancellation,
closed output, unsafe paths, workflow ownership and no-cruft replacement. Internal
tests inject faults only after observing the real lane-on publication boundary,
then check rollback state, retained files and owned staging cleanup. Exact-head
platform CI remains required before merge.

Focused lane and init-prompt acceptance passed in `31.952s`. The internal race
suite passed, configured lint reported `0 issues`, vet exited zero, gosec
reported `Issues : 0`, and govulncheck reported `No vulnerabilities found.`
The shell selftest reported `217 passed, 0 failed, 0 skipped`. Independent final
correctness/security/test review found no remaining actionable issues; the full
combined compiled-CLI regression and platform results are recorded in the PR.

## Rollout boundary

This is source implementation and integration. Existing installed script routes
remain available; installed Go activation, legacy script retirement and the
bounded ignored recovery lifecycle remain separate work.
