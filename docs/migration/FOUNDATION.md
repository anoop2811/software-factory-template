# Go runtime foundation candidate

This iteration starts the conversion specified in
[001-go-runtime-conversion.md](../../specs/001-go-runtime-conversion.md).
It supplies an immutable asset inventory and a developer-built Cobra dispatcher
with outside-in Ginkgo/Gomega compatibility coverage. It does not complete G0
or G1, migrate configuration/state, distribute releases, or retire legacy assets.
The existing `factory` entrypoint and all three native harness adapters remain
on their current scripts. Bash and the existing script prerequisites remain
required for this candidate.

## Try the candidate

From the factory source checkout, with the Go version in `go.mod`:

```sh
go build -o factory-go ./cmd/factory
./factory-go help
```

Keep the binary beside this checkout's `scripts/` directory. It delegates all
11 existing commands to those scripts and therefore carries their real effects;
for example, `init` and `upgrade` retain their existing write behavior. This is
a developer candidate, not an installer or a standalone distribution. The local
`factory-go` output is gitignored; remove it when finished.

## Acceptance and quality checks

`go test -race -count=1 ./...` runs external compiled-CLI and inventory scenarios.
The fixture dispatcher and asset tree come from immutable baseline
`76952eaa63aebd1ecd282f5ab51dd7c3627cb497`, not the mutable worktree.
Fetch full Git history when running the inventory suite from a shallow clone.
Fixtures use local script recorders; they do not call paid agents.

`make go-runtime-check` also runs formatting, the shared Go pack's Ginkgo gate,
vet, a temporary build, golangci-lint, gosec and govulncheck. Tool installation
versions are recorded in `.github/workflows/go-runtime.yml`; the same gate joins
`make check` in factory source checkouts. Adopters do not receive this module or
workflow and do not gain a Go prerequisite from this candidate.

The dialect rule and lint configuration remain shared with the Go pack. Only
the two blessed BDD packages are exempt from the dot-import style rule. Direct
process execution exceptions are scoped to the relevant calls with rationales.

## Next acceptance boundaries

The [inventory](COMPATIBILITY_INVENTORY.md) lists every remaining surface and
stage gate, including EX-001 and the release/platform/trust/pilot decisions.
Configuration and sourced-library parity need their own observed RED/GREEN
cycles. Artifact delivery and transactional migration/recovery must pass before
an adopter switches runtime. Local gitignored backup retention and predecessor
cleanup remain mandatory acceptance criteria for that delivery work.

## Observed development evidence (2026-09-06)

Before implementation, `go test ./acceptance -ginkgo.no-color` compiled the
candidate scaffold and reported:

```text
Ran 50 of 50 Specs in 12.268 seconds
FAIL! -- 12 Passed | 38 Failed | 0 Pending | 0 Skipped
```

Independent review then added seven execution/PATH/signal edge cases. The
focused run produced four passes and three behavioral failures before the
Bash compatibility fallback was implemented. Help mutation, missing/corrupt
inventory entries, rooted dialect violations and role-boundary probes provided
additional negative controls.

After implementation, `make go-runtime-check` exited 0 and included:

```text
ok  github.com/anoop2811/software-factory-template/acceptance 25.227s
0 issues.
Issues : 0
No vulnerabilities found.
```

`make eval` reported `PASS` for opencode, claude and codex structural wiring.
These are local deterministic checks. Live native-model runs, release installs,
backup/cleanup behavior and complete stage acceptance are not claimed here.
