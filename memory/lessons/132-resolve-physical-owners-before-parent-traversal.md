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

Observed 2026-11-03 UTC in PR #164's independent bare-operand probe: shells
located `sh mock.sh` through PATH but left `$0` bare. Resolving its parent from
the caller then executed a decoy factory. All five adapters reproduced the
failure. Do not assume `$0` identifies a caller-local file; the refusal contract
and retained local invocation are in
docs/adr/0133-go-source-evaluation-runners.md:154. The independent three-shell,
five-adapter matrix is retained under
`.factory/qualification/r3-runners/bare-owner-green-kvparglg/matrix.json`.
