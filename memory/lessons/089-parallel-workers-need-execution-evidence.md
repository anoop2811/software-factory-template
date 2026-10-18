# Parallel workers need execution evidence

An ordered Ginkgo container stays on one worker. Adding `--procs=2` alone cannot
distribute its scenarios. Split independent scenario groups with their own
preparation and cleanup, retain each interruption/recovery sequence intact, and
compare complete criterion identities before claiming coverage was preserved.

Prove that both workers execute criteria, rather than only checking runner flags.
A small independent suite can record distinct process IDs, exclusive per-criterion
receipts, JSON/JUnit outcomes and a deliberate worker failure. This separates
scheduler correctness from the full suite's performance measurement.

Provenance: observed 2026-10-18 via the race-enabled focused source-qualification
run (`go test -race -count=1 -timeout=3m -v ./acceptance
-ginkgo.focus='Go source qualification' -ginkgo.v -ginkgo.no-color
-ginkgo.fail-on-empty`): 12 passed, 0 failed. Canonical execution criteria are in
`acceptance/source_gate_parallel_test.go:59`; independent handoff groups are in
`acceptance/release_installation_test.go:252` and
`acceptance/release_installation_test.go:706`. The preparation and measurement
contract remains `docs/adr/0102-parallel-go-qualification.md:58`.
