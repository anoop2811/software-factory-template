# Own incidental discovery in feature fixtures

A feature fixture intentionally outside Git should own its ordinary non-Git
discovery result. Keep real discovery, timeout and process-ownership checks in
their dedicated tests; extending a production deadline would change that contract.

Provenance:

- [macOS base job 124660038324](https://github.com/anoop2811/software-factory-template/actions/runs/41242819739/job/124660038324) failed the existing empty-task eval criterion after 5.157 seconds. Its sanitized error does not establish the hosted root cause.
- Observed 2026-10-31 via a controlled delayed-ambient-Git Ginkgo overlay on that unchanged criterion: RED 0P/1F; after fixture isolation, GREEN 1P/0F with the ambient witness absent. The canonical no-overlay eval/discovery run passed 48 criteria.
- The fixture is in `acceptance/orchestration_eval_test.go:61`; ordinary nonzero fallback and the separate safety deadline are specified in `docs/adr/0077-go-command-environment.md:89`.
