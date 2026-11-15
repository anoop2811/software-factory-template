# Migration must survive the first drift check

Observed 2026-11-15 via the integrated fresh/v0.1.6 rehearsal in
`acceptance/integrated_adopter_rehearsal_test.go`. The ordinary upgrade and real
generated review passed, but the migrated clean checkout's healthy `factory
check` failed. Native sync reordered equivalent legacy Claude/MCP and OpenCode
JSON; the preserved Makefile correctly reported a Git diff.

The pre-correction focused run selected `initialized CI integrated adopter
rehearsal` with `go test -race -v -count=1 -timeout=25m ./acceptance`, and reported
`Ran 2 of 3497 Specs in 317.457 seconds`, `1 Passed | 1 Failed`. Its captured
diagnostic was `DRIFT: Claude config files do not match sync output`.

Do not normalize or resynchronize an upgraded fixture before committing its
public files: that can conceal the first-check failure an adopter will see.
Run the emitted checks from a clean clone and retain the original bytes.
Formatting compatibility must preserve meaningful policy order and real drift.
The contract and qualification boundary remain in
[ADR 0156](../../docs/adr/0156-integrated-adopter-rehearsal.md); this lesson does
not establish published-release or pilot qualification.
