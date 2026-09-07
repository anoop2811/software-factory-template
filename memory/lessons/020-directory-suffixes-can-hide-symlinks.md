# Check directory suffixes before rejecting symlink roots

On the supported Unix environments, a trailing slash or `/.` makes `Lstat`
observe a directory symlink's target. Rejecting only the unsuffixed spelling
does not establish a no-symlink store boundary. Strip terminal separators and
dot components before checking the store, while preserving interior `..`
semantics: cleaning those can select a different directory when an earlier
component is a symlink.

Provenance: observed 2026-09-07 through independently written CLI regressions
in `acceptance/selection_test.go` for the Decision 54 selector. Command
`go test -count=1 ./acceptance -ginkgo.no-color -ginkgo.focus='rejects a symlink store with literal path suffixes'`
reported `0 Passed | 2 Failed`: both symlink suffixes returned 0 instead of 1.
After terminal-suffix normalization, the selection suite reported
`ok .../acceptance 8.813s`. The contract remains in ADR-0059.
