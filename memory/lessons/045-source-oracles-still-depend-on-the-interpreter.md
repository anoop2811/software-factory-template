# Source oracles still depend on the interpreter

An immutable application commit does not freeze its standard library. A
differential parser test can pass locally and fail in CI because the same
application source runs under different interpreter patch releases. Record the
interpreter boundary and characterize any explicit exception separately from
the candidate's fixed contract; never make candidate assertions accept whichever
result happens to pass.

Provenance: observed 2026-09-25 UTC in PR #99's Linux source job,
https://github.com/anoop2811/software-factory-template/actions/runs/36081983647/job/107905725836.
The sole failure was mixed short-help `-hj`: Go and local Python 3.12.0 refused,
while the CI oracle returned help. Executing tagged CPython argparse sources
isolated the behavior change between v3.12.2 and v3.12.3. Canonical qualification:
docs/adr/0072-go-budget-argument-compatibility.md; command/output evidence:
docs/migration/BUDGET_ARGUMENTS.md.
