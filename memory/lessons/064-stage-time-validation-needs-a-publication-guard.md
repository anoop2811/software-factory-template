# Validate related inputs at publication time

A transformation callback runs before its output is staged. When another file
determines that transformation, checking it only in the callback can miss a
replacement during staging. Use the shared writer's final publication guard;
the governing contract is docs/adr/0090-go-native-config-migration.md:65.

Provenance: observed 2026-09-30 via the migration guard negative control.
Temporarily omitting the guard made `go test ./internal/migrateconfigcmd
-ginkgo.focus='refuses observed legacy replacement after YAML staging'
-ginkgo.no-color -ginkgo.succinct -count=1` report `0 Passed | 2 Failed`
(`0.369s`); restoring it passed (`0.285s`). The controlled fixture observes the
real staged file before changing legacy input; see
internal/migrateconfigcmd/publication_test.go:59. This establishes the tested
staging boundary, not a concurrent-writer atomic transaction.
