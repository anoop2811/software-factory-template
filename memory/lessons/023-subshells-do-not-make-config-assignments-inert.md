# Subshells do not make configuration assignments inert

Provenance: observed 2026-09-07 via the local-hook security reproduction and
`acceptance/hooks_test.go` inherited-attribute regression; contract in
`docs/adr/0062-go-local-hook-normalization.md`.

A helper ran in a subshell to preserve caller state, but a named scratch variable
inherited Bash's integer attribute. Assigning literal configuration containing
`array[$(touch marker)]` evaluated the arithmetic subscript and created the marker.
The subshell prevented parent-variable changes, not execution.

For sourceable adapters, treat shell variable attributes as part of the boundary.
The local-hook adapter uses positional data transport instead of assigning
untrusted text to named scratch variables. See the ADR and regression for the
canonical contract; do not infer that every existing adapter has this protection.
