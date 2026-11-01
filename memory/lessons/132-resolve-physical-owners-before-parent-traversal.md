# Resolve physical owners before parent traversal

`pwd -P` cannot recover an owner after logical `cd alias/scripts/..` has already
collapsed the symlinked component. Traverse physically before interpreting `..`.
Keep a decoy executable in the alias parent so the acceptance check observes
which owner actually runs, alongside literal argument/input preservation.

Provenance: observed 2026-11-01 UTC in the independent directory-alias RED:
physical init/upgrade returned 63; both directory aliases executed the decoy and
returned 64. Current `cd -P` controls subsequently passed. The criterion is
acceptance/source_entrypoints_test.go:308; its contract is
docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:67. Exact commands and
receipts are retained locally under
`.factory/qualification/source-entrypoints-20261101/source-directory-alias-red-freeze.json`.
