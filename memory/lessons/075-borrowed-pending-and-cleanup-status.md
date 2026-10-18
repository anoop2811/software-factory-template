# Borrowed pending and cleanup status

A constructor can borrow a pending marker without granting new publication
authority. If it refuses, reuse resource closure without reporting that borrowed
marker as its own unfinished grant. A guard validation conflict also differs
from a failed descriptor close; inspect every joined error before giving an
operational cleanup error precedence.

Canon: docs/adr/0096-interrupted-publication-recovery.md:224 and
docs/adr/0096-interrupted-publication-recovery.md:276. This lesson points to that
contract rather than defining another recovery policy.

Provenance: observed 2026-10-05 UTC through independent actual Git refusals,
late activity creation, descriptor/flock closure and unchanged regression
rechecks recorded in Decision 95 of docs/DECISION_LOG.md. Read back
/private/tmp/factory-r22-git-refusal-red.log and
/private/tmp/factory-r22-close-open-red.log before correction, then
/private/tmp/factory-r22-close-open-final.log (eight passed, package 5.728s).
These are source-test observations, not released customer incidents.

Follow-up provenance: Copilot comment 4180316394 on PR #123 exposed the same
first-match status risk across the constructor and cleanup trees. Observed
2026-10-05 UTC: ten independent native-operation cases first returned 6 passed
and 4 failed, then 10 passed without evaluator changes after the private join
correction. Exact commands/output are recorded in Decision 95 of
docs/DECISION_LOG.md; canon is
docs/adr/0096-interrupted-publication-recovery.md:291. The refinement covers both
branches while retaining cancellation and typed ownership evidence.

Follow-up provenance: observed 2026-10-18 UTC through the public candidate
upgrade criterion in acceptance/release_installation_test.go:559. A canceled
private ignore rule preserved a prepared journal and concrete recovery inputs,
but the process returned 1 instead of policy-refusal status 2 because deliberate
cache retention was represented as a cleanup error. The independent RED ran
`go test -race -count=1 -timeout=7m -v ./acceptance -ginkgo.focus='preserves policy-refusal status while retaining prepared recovery inputs' -ginkgo.no-color -ginkgo.fail-on-empty`:
`0 Passed | 1 Failed`, package `124.138s`. The refined contract is
in docs/adr/0101-go-release-handoff.md:579: report retained input locations
separately from actual cleanup failures. This is a source regression, not a
released customer incident.
