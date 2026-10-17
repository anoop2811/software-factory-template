# Orchestration parity includes selection and working directory

Observed 2026-10-17 through independent compiled Go versus immutable 4ecab796
Bash reproductions during the remaining-orchestration review: a correct harness
argv is insufficient if a named custom runner is bypassed, its working directory
changes, or a different repository supplies its task/configuration. One error
could bypass a local cost guard; another skipped a rejecting local hook beneath
an explicit nested REPO_ROOT. Tests should observe which artifact executes and
where, alongside its arguments and result. Canonical contracts and corrections
remain in docs/adr/0099-native-factory-orchestration.md:97 and
docs/adr/0099-native-factory-orchestration.md:205.

Observed in the real check cancellation fixture on the same date: Darwin's
foreground-terminal query on inherited `/dev/null` returned ENODEV, before the
gate could start. A noninteractive descriptor needs its own control case; a
regular-file stdin scenario did not expose it. The platform handling is recorded
in docs/adr/0099-native-factory-orchestration.md:197, not generalized into a rule
that all terminal-query errors can be ignored.

Provenance: review reproductions retained locally at
`/private/tmp/factory-review-check-root-ed58lrdx`,
`/private/tmp/factory-review-verifier2-596fiao1`, and
`/private/tmp/factory-check-stdio-debug.go`; the regression cases live in
`acceptance/orchestration_check_test.go` and
`acceptance/orchestration_eval_test.go`.
