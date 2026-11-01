# Cross-compile platform test helpers

A product binary can build while its platform test helper fails compilation.
Use the portable wrapper already present in the dependency lock when the
standard syscall package differs by architecture. Cross-compile the acceptance
package for every supported target before pushing a low-level helper change;
compilation does not establish native execution.

Provenance: observed 2026-11-01 UTC in the [Linux arm64 bundle job](https://github.com/anoop2811/software-factory-template/actions/runs/41339009796/job/124944826163):
the product built but acceptance compilation reported `undefined: syscall.Dup2`.
A local compile reproduced that failure. After using the existing locked
`unix.Dup2`, all four Linux/macOS amd64/arm64 acceptance cross-compiles passed.
The contract is docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:252;
exact commands and results remain local in
`.factory/qualification/source-entrypoints-20261101/qualification.json` under
`source_wait_portability`.
