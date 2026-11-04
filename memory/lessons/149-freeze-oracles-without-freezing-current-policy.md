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

Audit whole-tree fixture materializers too: an old dispatcher plus a new adapter
can form an exec loop, even when the tested feature is disabled. PR #166's
budget, loop and Java fixtures exposed this because the old initializer always
checks review-lane pending state. The corresponding immutable review commands
must accompany that initializer. Provenance: observed 2026-11-04 UTC in the
[budget CI failure](https://github.com/anoop2811/software-factory-template/actions/runs/41851815573/job/126649822508)
and independent bounded exec trace; corrected private runs of
`bash scripts/selftest/budget.sh`, `bash scripts/selftest/loop.sh` and
`bash scripts/selftest/java-build-tool.sh` reported 32, 40 and 6 passed,
respectively, with zero failures. No timeout increase was needed.
