# Freeze historical command oracles without freezing current policy subjects

During a source cutover, one fixture tree can contain both old command engines
and current policy checks. Replacing a library at that shared root can silently
make the current checks exercise the old policy dependency too. Keep historical
command dependencies in their own fixture scope. Where current and historical
consumers temporarily share identical bytes, assert that identity and require
explicit isolation when the current implementation changes.

Provenance: observed 2026-11-04 UTC during independent review of the R3 review
cutover. The initial optional-pack fixture overlaid historical config/color at
its shared root. It now retains current library bytes and refuses divergence
from the declared historical revision before running the mixed fixture.
Changing the private fixture's config reader produced status 1 with
`isolate changed historical review dependency: scripts/lib/config.sh`;
the unmodified fixture then reported `optional-packs: 3 passed, 0 failed`.
Design and custody authority: docs/adr/0135-go-source-review-entrypoints.md:46
and docs/migration/R3_PROOF_RETIREMENT.md. This lesson points to that contract;
it does not add a new runtime fallback or freeze policy gates.
