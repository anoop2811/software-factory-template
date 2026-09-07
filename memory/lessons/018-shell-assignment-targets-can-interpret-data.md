# Shell assignment targets can interpret data

Avoiding eval is necessary but insufficient when applying configuration to Bash
variables. Integer attributes trigger arithmetic evaluation, and namerefs redirect
assignments and attribute changes. Validate supported target attributes before
applying any record, including export-only records. Checking each target only as
it is assigned can leave earlier records applied before a later refusal.

Provenance: GNU Bash builtin documentation, fetched 2026-09-07 UTC:
https://www.gnu.org/software/bash/manual/html_node/Bash-Builtins.html

The Go candidate's admission boundary is recorded in
docs/adr/0056-go-configuration-export-plans.md:73. Acceptance scenarios under
`G1 sourceable configuration export` exercise arithmetic-bearing integer values
and unsupported target attributes; the legacy public compatibility decision
remains separate from candidate refusal.
