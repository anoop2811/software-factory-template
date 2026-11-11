# Qualify consumers when shared workflow test models change

Adding a YAML field to a shared test struct can break a consumer that inlines
the struct and declares the same field itself. The compiler accepts that shape,
but the YAML decoder panics on the duplicate tag. A passing test of the shared
model's primary consumer does not cover its other decoding consumers.

Search embeddings and decode sites when extending shared test models. Keep one
field declaration and preserve its original presence semantics; changing a
pointer to a string can silently turn an absent-field check into an empty-value
check. The correction's contract remains in ADR 0149, rather than this lesson.

Provenance: observed 2026-11-11 UTC in
[PR #179's Linux source-entrypoint job](https://github.com/anoop2811/software-factory-template/actions/runs/42966125427/job/130343674464)
and its macOS counterpart. The exact structural-workflow criterion reproduced
the duplicate-`shell` panic locally before correction.
See `docs/adr/0149-compiler-provenance-and-security-patches.md:201`,
`acceptance/source_gate_selection_test.go:813`, and
`acceptance/source_structural_entrypoint_test.go:298`.
