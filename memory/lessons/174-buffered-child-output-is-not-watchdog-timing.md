# Buffered child output does not establish watchdog timing

A captured completion message does not prove that the direct command completed
before its watchdog. An independently supervised child can inherit output pipes
and finish after CommandContext kills the direct caller. Keep the context failure
and capture elapsed time, the command error and actual process exit state before
attributing the failure to shutdown or weakening its deadline.

Observed 2026-11-12 via GitHub Actions run `43305598632`, macOS local-upgrade job
`131487951720`: the versioned Go predecessor case failed the fresh 90-second
watchdog while its buffered output included `installation completed`. Ten other
criteria passed; the failure occurred about eight minutes into the suite, not at
its aggregate limit. Source inspection of `acceptance/release_installation_test.go`
at commit `6d64d6d81e1529459a11029266fdbb38d11a4f71` shows default direct-process
cancellation and inherited child output capture. This evidence does not establish
a production post-completion hang.

[ADR 0148](../../docs/adr/0148-transactional-installed-ci-migration.md) records
the workload-isolation decision and its limits. Actual local-source compilation
remains part of the proof; fixture reuse must not replace it with packaged bytes.
