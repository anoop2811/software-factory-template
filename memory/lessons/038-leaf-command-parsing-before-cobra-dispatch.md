# Parse a private leaf before Cobra command discovery

Observed 2026-09-23 via the independently authored budget command hidden
completion regressions: disabling the default completion command still allowed
`__complete` to print completion output before the adapter returned status 2.

A private leaf can parse its flags and reject remaining positional operands
before ExecuteContext receives an empty argument slice. This preserves literal
scalar values while preventing hidden command dispatch. Do not substitute a
blanket token blacklist: completion-looking filenames can be valid data.

Provenance: docs/adr/0071-go-budget-command-candidate.md:28 defines the refusal
contract; acceptance/budget_command_test.go contains hidden completion and
completion-looking filename cases. The focused final race run is recorded in
docs/migration/BUDGET_COMMAND.md. This lesson points to that contract, not a
second command-parser specification.
