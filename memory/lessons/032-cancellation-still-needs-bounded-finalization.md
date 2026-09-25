# Cancellation still needs bounded finalization

Canceling native execution does not remove the obligation to persist its outcome.
Reusing the canceled context for ledger finalization can leave a stopped process
looking active; using an unbounded background context can hang recovery. Detach
cancellation only for an explicit, bounded cleanup phase and retain the original
PID/publication error if cleanup cannot establish durable completion.

Provenance: `docs/adr/0070-go-budget-execution-controller.md:76` defines the
five-second cleanup contract. https://pkg.go.dev/context#WithoutCancel and
https://pkg.go.dev/context#WithTimeout were fetched 2026-09-22 UTC to confirm
context behavior. Ownership and publication rules remain canonical in the ADR;
this lesson does not claim installed runtime activation or arbitrary filesystem
syscall cancellation.
