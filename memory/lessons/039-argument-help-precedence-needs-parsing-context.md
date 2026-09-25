# Help precedence depends on parsing context

Observed 2026-09-23 via the immutable budget oracle and compiled-CLI differential
cases in acceptance/budget_arguments_test.go: a global search for a help token
cannot reproduce argparse behavior. Earlier missing values/invalid choices can
defeat help; ambiguity anywhere in the current parser scope also defeats help,
while unknown operands can be deferred and superseded by help.

The factory's contract is recorded in
docs/adr/0072-go-budget-argument-compatibility.md:37. Its implementation derives
option identity/arity from registered Cobra flags, classifies before processing
actions, and keeps metadata validation in its original domain. Future parser
conversions should characterize precedence at the real entrypoint rather than
assuming two CLI libraries treat the same tokens equivalently.

Provenance: the seven core cases failed before production changes; the final
109-case instrumented run and normalization boundaries are recorded in
docs/migration/BUDGET_ARGUMENTS.md. This lesson does not replace that contract.
