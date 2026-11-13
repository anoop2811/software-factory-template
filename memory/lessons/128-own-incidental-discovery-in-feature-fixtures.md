# Own incidental discovery in feature fixtures

A feature fixture intentionally outside Git should own its ordinary non-Git
discovery result. Keep real discovery, timeout and process-ownership checks in
their dedicated tests; extending a production deadline would change that contract.

Provenance:

- [macOS base job 124660038324](https://github.com/anoop2811/software-factory-template/actions/runs/41242819739/job/124660038324) failed the existing empty-task eval criterion after 5.157 seconds. Its sanitized error does not establish the hosted root cause.
- Observed 2026-10-31 via a controlled delayed-ambient-Git Ginkgo overlay on that unchanged criterion: RED 0P/1F; after fixture isolation, GREEN 1P/0F with the ambient witness absent. The canonical no-overlay eval/discovery run passed 48 criteria.
- The fixture is in `acceptance/orchestration_eval_test.go:61`; ordinary nonzero fallback and the separate safety deadline are specified in `docs/adr/0077-go-command-environment.md:89`.

The same incidental boundary recurred in the source-runner qualification on
2026-11-13 UTC: five fixtures refused evaluation before the golden header, and
the four cancellation receipts had no admitted harness PID/group. Reuse the
existing discovery fixture instead of duplicating a near-equivalent stub. The
receipts establish preparation refusal, not the underlying host Git cause.

Additional provenance: [macOS source-entrypoint job 131658583969](https://github.com/anoop2811/software-factory-template/actions/runs/43356159724/job/131658583969),
observed with its retained JSON report; the bounded fixture decision is in
`docs/adr/0148-transactional-installed-ci-migration.md:683`. Behavioral
qualification of this follow-up is recorded with the decision receipts.
