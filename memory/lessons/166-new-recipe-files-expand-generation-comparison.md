# New recipe files expand generation comparison

Adding a frozen historical generation can change the current selection algorithm
even when its runtime role rows stay unchanged. If the new source file defines
recipe fingerprints or historical roles, include that file in the comparison
that claims two images share a generation. Otherwise a predecessor differing only
there can skip historical qualification and be interpreted through current code.

Keep existing frozen fingerprints immutable. Test an unchanged image as a healthy
control, then change only the new generation source and require refusal. A test
that merely reads the new fingerprint literals cannot expose this omitted input.

Provenance: `internal/installation/recipe.go:79`,
`internal/installation/recipe_compatibility_test.go:126`, and
`docs/adr/0148-transactional-installed-ci-migration.md:81`. Observed 2026-11-11
via the independent focused race regression: the changed-only predecessor
returned status 0 before the comparison correction; the healthy control passed.
The ADR remains the contract; this lesson records the missed dependency.
