# Native Go cost report

Decision 86 and [ADR-0087](../adr/0087-go-native-report.md) define source-native
`report`. The Cobra command computes gate counts, event records and a labeled
review-cost estimate without `scripts/factory-report.sh` or a sourced config
library. Configuration export planning reuses the existing parser. A shared
bounded descriptor reader serves both report and doctor.

The dispatcher directory selects installed hooks and the default event log.
Configuration discovery remains relative to the caller/Git checkout; explicit
configuration and event-log overrides retain their established meanings.
`--clear` is recognized only as the first argument. It retains best-effort
unlink semantics, preserves directories and never follows the final symlink.
Ordinary reporting does not write project files or invoke a model.

Explicit corrections are documented in the ADR: empty logs produce one zero,
estimates use checked decimal arithmetic, newline-bearing hook names count once,
and NUL-bearing event logs are rejected instead of depending on Bash version.
Missing configuration is allowed. Missing event logs mean zero records; unsafe,
oversized or unreadable inputs produce a diagnostic and nonzero status.

## Qualification

The independent native-route tests failed before production with `0 Passed |
3 Failed`: the executable still required the missing legacy script. The initial
implementation passed those three cases in `3.113s`. Subsequent compatibility
and safety cases are post-implementation checks, not additional claimed RED
cycles. The frozen ordinary-output reference is commit
`9a3f2e4c61bbc92e4b05fde517cd60e78c4b7165`.

Local safety qualification used both an instrumented compiled CLI and an
instrumented test runner. The initial safety matrix passed in `12.940s`;
supplemental SIGINT/SIGTERM and closed-output-pipe cases passed in `3.145s`.
The shared parser/doctor regression selection passed in `47.390s`.
The final report and dispatcher acceptance selection with both CLI and outer
race instrumentation passed in `40.145s`, including nine frozen parity cases.
Configured acceptance lint reported `0 issues`; repository vet passed, gosec
reported `Issues : 0`, and govulncheck reported `No vulnerabilities found.`
The shell selftest reported `217 passed, 0 failed, 0 skipped`. Independent
production and test reviews reported no remaining actionable findings.
Exact-head platform CI remains a separate gate before merge.

## Rollout boundary

This change completes source implementation of report, not installed activation.
The existing shell report is retained for script-based installations. Legacy
retirement and the ignored, bounded recovery lifecycle remain prerequisites to
the public Go cutover; a native report does not authorize deleting old scripts.
