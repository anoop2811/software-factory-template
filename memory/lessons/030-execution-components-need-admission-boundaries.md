# Execution components need admission boundaries

Porting a subprocess executor does not port the budget controller that calls it.
A private CLI launch route can still bypass reservation and durable PID
publication. Keep the executor internal until the controller supplies those
steps, and exercise it through test-only drivers while integration is pending.

Provenance: the boundary is specified in
`docs/adr/0068-go-native-harness-execution.md:20` and the callback ordering in
`docs/adr/0068-go-native-harness-execution.md:36`. The immutable implementation
source is commit `4cc771e894e11d5024032106aeb0ef788997bd29`,
`scripts/lib/budget.py`, whose `run` supplies admission around `execute`.
This lesson points to that contract; it does not qualify installed execution.
