# Preflight failure and process ownership

A failed readiness check does not by itself establish uncertain process exit.
A missing CLI or unsupported flag should permit a later manual check when
accounting is readable and no active process is owned. Conversely, an empty
budget ledger does not prove exit: local capability probes can start before
budget admission. Carry unconfirmed probe ownership explicitly across that
boundary, including failures during response parsing.

The contract remains in docs/LOOPS.md and Decision 47; this lesson points to it.

Provenance: PR #72 review read 2026-09-06,
https://github.com/anoop2811/software-factory-template/pull/72#discussion_r3945498221;
observed 2026-09-06 by reading scripts/lib/budget.py at commit 7f8bc82
(preflight precedes admission) and scripts/lib/budget_adapters.py at that commit
(probe cleanup can outlive a parsing exception).
