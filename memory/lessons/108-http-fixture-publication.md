# Publish fixture state before an HTTP handler reads it

A handler closing over a service pointer needs a Go synchronization edge after
that pointer is assigned. Starting a later CLI request was insufficient in the
review fixture: Linux race detection found three unsynchronized captures despite
every behavior assertion passing. A closed ready channel now publishes each
pointer before the handler reads it; keep the behavioral assertions unchanged.

Provenance: observed 2026-10-29 via
[the failed Linux qualification](https://github.com/anoop2811/software-factory-template/actions/runs/40945885808/job/123700983698).
The canonical execution and qualification contract remains
`docs/adr/0112-bounded-parallel-review.md:54`.
