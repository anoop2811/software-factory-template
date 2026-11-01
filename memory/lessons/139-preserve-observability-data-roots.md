# Preserve command ownership separately from the measured project

A direct command adapter can resolve its executable physically without changing
the caller's working directory. Report reads default events beside its owner;
metrics measures the caller's project and borrows only its HTML template from
the owner. A shared adapter that changes cwd would collapse these distinct roots.

Provenance: observed 2026-11-01 through the independently authored
`Go source runtime entrypoints observability adapters` criteria: the old scripts
failed all three criteria, then the two thin adapters passed all three. The
native-output case gives owner and caller different event counts and poisons
the shell libraries. See acceptance/source_observability_entrypoints_test.go:37,
docs/adr/0087-go-native-report.md:22 and docs/adr/0088-go-native-metrics.md:27.
