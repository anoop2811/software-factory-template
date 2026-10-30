# Split changes at complete function boundaries

A small file count is not enough: an adapter and the dispatcher it calls must
form a working path together. Replacing a script with a call back to the same
old script dispatcher creates recursion. Likewise, retire shared controllers
only after their last consumer and proof runner migrate. Preserve a committed
integration reference while delivering functions with their own tests and docs.

Provenance: the operator requested a functional split of PR #141 on 2026-10-29;
dependency review inspected the
[original script dispatcher](https://github.com/anoop2811/software-factory-template/blob/61e184470bcb17574e61aba86b8bd4eacc1fdc70/factory#L43).
The canonical delivery and compatibility constraints are in
`docs/adr/0114-incremental-go-cutover-delivery.md:15`.
