# Audit opt-in evaluator process custody

Observed 2026-11-15 during RC4 preflight by two independent source reviews of
the `publishedrelease` evaluator at commit
`509d4bbc01743fa3900247273d082171e1bf3a18`,
`acceptance/published_release_test.go:508` and `:207`. Its explicit crash
observer still combined a concurrent wait with later numeric signals; its
ordinary command helper used a raw group-signal cancellation callback.
The shared owner already addressed these patterns elsewhere in the source
suite. This was a source-review finding, not an observed customer incident.

The pinned Go 1.27.4 source, read from the qualified local toolchain on the same
date, places `Process.Wait` at `src/os/exec/exec.go:944` before the context-watcher
join at `:952`; the watcher can invoke the callback at `:816`. A custom raw
numeric signal does not acquire the protections of Go's process methods.

When a lifecycle correction lands, include explicitly tagged evaluators in the
call-site audit and run their selected controls. An ordinary green suite does
not show that a separate live evaluator adopted the correction. Keep actual
release qualification distinct from compilation, selection and helper proofs.
The contract and bounded correction are in Decision 173 in
[the decision log](../../docs/DECISION_LOG.md); the shared ownership contract
remains [ADR 0151](../../docs/adr/0151-reliable-ci-qualification.md).

Observed 2026-11-16 in PR #184's
[macOS historical-recipe job](https://github.com/anoop2811/software-factory-template/actions/runs/43775569321/job/133002624455):
the retained `obsolete-cache.actual-interruption` entry records a real applying
journal receipt, then exit 2 with `fatal error: all goroutines are asleep - deadlock!`
at the injected `independentObsoleteBoundary` nil-channel receive. The raw
installation route uses a background context; its `Done()` channel cannot serve
as an external wait. A test pause must keep a real pollable wait alive while
preserving the signal behavior it intends to measure. Decision 173 records this
evaluator correction separately from the production migration contract.
