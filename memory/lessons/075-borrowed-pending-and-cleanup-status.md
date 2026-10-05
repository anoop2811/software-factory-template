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
