# CLI help has an independent byte contract

Adding an approved help line requires updating the independent native-help
fixture and running the existing command-boundary suite. Do not weaken exact
output comparison or normalize away the addition. The legacy help fixture is a
separate contract and remains unchanged.

Provenance: observed 2026-10-30 UTC in Actions job `124222059512` at source
`ae025b46391fb9a19c4db4f95aeb5fcb2a2a19cb`. All seven help aliases differed
by exactly the newly advertised local-init line. Local reproduction failed all
seven; the corrected fixture then passed all 27 command-boundary criteria.
`acceptance/cli_test.go:32` owns the expected native text;
`internal/cli/cli.go:41` owns the implementation text. The display addition is
authorized in `docs/adr/0116-explicit-go-local-initialization.md:168`.
