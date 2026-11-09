# Disabling cancellation does not preserve pending workflow runs

GitHub Actions can replace an older pending run when another run enters the same
concurrency group, even with `cancel-in-progress: false`. Use a unique run-ID group
when each non-PR invocation must remain independently eligible to run; reserve
shared PR-number groups for deliberately superseded pull-request checks.

Provenance: fetched 2026-11-09 from the official
[concurrency documentation](https://docs.github.com/en/actions/how-tos/write-workflows/choose-when-workflows-run/control-workflow-concurrency).
The project decision is in
[ADR 0145](../../docs/adr/0145-installed-ci-workflow-rendering.md#event-and-range-qualification-refinement).
