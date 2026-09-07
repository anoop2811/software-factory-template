# ADR-0053: advertise budgeting, observability and loop engineering

Status: accepted for implementation, 2026-09-07 UTC.

The user requested landing-page coverage of the shipped budget, observability
and loop features. Add a prominent section to index.html, linked from the main
navigation, using the existing responsive card and command-example styles.
Explain developer benefits, opt-in choices and native Codex/Claude/OpenCode
support, with links to the canonical guides rather than a second feature spec.

Budget claims follow docs/BUDGETS.md:8: only factory-managed invocations are
controlled. Attempt, session time and concurrency limits stop further launches
(docs/BUDGETS.md:83). Reported USD is an estimate, unknown cost remains unknown,
and one invocation may cross an estimate threshold (docs/BUDGETS.md:91).
Local plan/report commands do not invoke a model (docs/BUDGETS.md:14).
The ledger metadata promise excludes native history (docs/BUDGETS.md:107).

Loop claims follow docs/LOOPS.md:16 for the manual default and docs/LOOPS.md:42
for explicit bounded implementation/check/review/repair. The loop uses shared
budget accounting (docs/LOOPS.md:51). Mention finite stop conditions and handoff
(docs/LOOPS.md:59). Do not advertise terminal bounded checkpoints as automatic
paid resume (docs/LOOPS.md:147).

Show local preview/report examples, not a paid launch disguised as a preview.
Link bounded-mode setup, including reviewer accounting, to docs/LOOPS.md:107.
Retain the existing metrics section for gate-level observability.

Release availability: the official GitHub release API reports v0.1.6 as latest
(published 2026-09-06T19:10:47Z), read 2026-09-07 UTC via gh release view. The tag
contains neither docs/BUDGETS.md nor docs/LOOPS.md (git ls-tree -r --name-only
v0.1.6 docs/BUDGETS.md docs/LOOPS.md returned no paths). Label these additions as
available on main and link the existing --ref main install/upgrade instructions;
do not imply that the current default release includes them.

Validate the changed page in a real browser at desktop and mobile widths,
check navigation/document links and JavaScript errors, and run the existing
landing-page/template checks. No new runtime dependency or behavioral test
suite is needed for this static content change.
