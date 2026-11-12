# Compiler pins can block artifact handoff

A reader that compares artifact compiler provenance with its own producer pin
can reject a security-patch release before its new executable gets control.
Updating only the new reader cannot repair an already published old reader.
Reproduce both directions: historical input into the new reader, and the actual
old reader attempting the new input. A successful historical-inspection test
alone does not establish forward handoff compatibility.

Keep producer reproducibility and artifact identity as separate concerns. The
contract and authenticated rebootstrap remedy live in ADR 0149; this lesson
records why a compiler patch exposed an upgrade-path dependency.

Provenance: `docs/adr/0149-compiler-provenance-and-security-patches.md:28`,
`acceptance/predecessor_recipe_compatibility_test.go:181`, and
`internal/staging/source_identity.go:53`. Observed 2026-11-11 UTC via the genuine
historical-reader/fresh-installer race criterion: the immutable Go 1.27.3 reader
refused Go 1.27.4 metadata; the fresh authenticated owner produced a clean preview.
