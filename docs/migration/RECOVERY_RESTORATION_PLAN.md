# Read-only recovery restoration planning

[ADR-0092](../adr/0092-go-recovery-restoration-planning.md) defines the
source-built Go restoration planner. It compares one checked local recovery
set against the same selected installed reference paths. It reports candidates
and conflicts without restoring, deleting or changing any file.

## Inspect the proposed scope

Run from the physical installation root with a locally built Go binary:

```sh
go build -o factory-go ./cmd/factory
/path/to/factory-go upgrade --plan-restore --migration-id migration-001
/path/to/factory-go upgrade --plan-restore --migration-id migration-001 --json
```

A completed plan returns status 2 because restoration is still ineligible.
A bounded observation or output failure returns status 1. Invalid arguments,
unsafe roots and observations changed while planning refuse with a safe
diagnostic. No source argument, confirmation or execution configuration is
needed. Planning does not require Git and does not invoke the legacy upgrader.

The selected set determines the candidate scope; the command does not inspect
other sets or expand the six-path catalog. An intact held set is readable and
its hold is reported. Missing, incomplete, unrecognized and unsafe sets produce
no file candidates, remain preserved and show why inspection cannot proceed.

The per-file actions are:

| Action | Meaning |
|---|---|
| retain_reference | Existing ordinary content and complete mode match the reference. |
| restore_missing_candidate | A path is absent; its absence does not prove migration ownership. |
| preserve_customized | Later ordinary content or mode differs and must be kept. |
| conflict | The path, type, link, ownership or permissions are unsafe. |
| assessment_error | A bounded observation could not complete. |

The report includes the recovery classification, hold, counts, reference
metadata, current observations and explicit blockers. No file contents or
absolute installation roots are printed. Text and JSON carry the same
information; zero-valued action counts remain present.

## Why application remains blocked

Saved reference bytes prove integrity. They do not prove that a completed
migration removed a missing path, that a current replacement remains unchanged,
or that the old runtime can safely consume today's state. Current admission
locks also cannot establish that an older process paused before admission is
absent. Restoring executable files before those conditions are established
would bypass the conversion specification's rollback safeguards.

Consequently every plan retains ownership, transition-quiescence, compatibility
and activation-check blockers. All applicability, restoration, rollback,
activation and pruning authority fields remain false. A missing candidate is
information for a future qualified operation, not permission to recreate it.

Planning never creates locks or folders, rewrites ignores, updates version
metadata, launches processes, runs checks or reads/resets runtime history.
Existing installed files, backups, configuration, Git index and history remain
unchanged. The installed shell dispatcher remains unchanged.

## What remains

Controlled restoration needs reverse transaction ownership, a qualified
runtime transition boundary and compatibility/activation evidence. Full legacy
inventory, transactional activation, removal of redundant active scripts and
later-release retention remain separate required work. This planner does not
make upgrades from v0.1.6 release-ready and earns no additional credit toward
the controlled-restoration milestone.

The separate [live publication core](LIVE_RESTORATION.md) records ownership from
an actual same-process replacement. Its qualification is tracked separately;
it does not grant this planner restoration authority or enable a public command.

## Qualification

Independent outside-in RED preceded production implementation. The spec-writer
ran the compiled command against unchanged production source:

```text
FACTORY_AGENT_ROLE=spec-writer GOCACHE=/private/tmp/factory-durable-recovery-go-cache go test -v ./acceptance -ginkgo.focus='Read-only recovery restoration planning core' -ginkgo.no-color -ginkgo.succinct
Ran 4 of 2249 Specs in 3.084 seconds
FAIL! -- 0 Passed | 4 Failed | 0 Pending | 2245 Skipped
FAIL github.com/anoop2811/software-factory-template/acceptance 3.604s
```

Creation fixture setup passed; the new planning requests reached ordinary
legacy dispatch and returned no structured plan. Production work began only
after these four observed failures. Independent review then exposed incorrect
attribution of late installed changes and a status-priority regression. The
spec-writer observed nine and two additional failing lifetime controls before
the corresponding corrections; neither finding remains open.

Pre-publication compiled-command qualification on 2026-10-03 selected 73 acceptance
specs, including 66 public planning cases and seven existing help cases. The
outer test process and compiled child both used the race detector:

```text
FACTORY_AGENT_ROLE=reviewer FACTORY_CLI_TEST_RACE=1 GOCACHE=/private/tmp/factory-durable-recovery-go-cache go test -race -count=1 -v ./acceptance -ginkgo.focus='Read-only recovery restoration planning|preserves help aliases without adding framework commands' -ginkgo.no-color -ginkgo.succinct
ok github.com/anoop2811/software-factory-template/acceptance 52.984s
```

All 73 selected specs passed; none of the selected specs skipped. The independent
spec-writer also ran the complete two affected internal suites under the race
detector after the final correction:

```text
FACTORY_AGENT_ROLE=spec-writer GOCACHE=/private/tmp/factory-durable-recovery-go-cache go test -race -v ./internal/assessment ./internal/recoverycmd -ginkgo.no-color -ginkgo.succinct
139/139 assessment specs: SUCCESS! 8.622628166s
26/26 recovery-command specs: SUCCESS! 4.45245825s
```

The counts above identify selected suite results, with progress dots omitted.
A broader compiled race check selected 206 existing creation, recovery-inventory
and installation-assessment specs: 202 passed and four host-dependent fixtures
skipped. It ran before the final planning-only status correction:

```text
FACTORY_AGENT_ROLE=reviewer FACTORY_CLI_TEST_RACE=1 GOCACHE=/private/tmp/factory-durable-recovery-go-cache go test -race -count=1 -v ./acceptance -ginkgo.focus='Durable local recovery creation|G4 recovery inventory|G4 installation assessment' -ginkgo.no-color -ginkgo.succinct
ok github.com/anoop2811/software-factory-template/acceptance 117.159s
```

Pre-publication whole-source quality checks used the existing qualified tools and caches:

```text
GOCACHE=/private/tmp/factory-durable-recovery-go-cache GOLANGCI_LINT_CACHE=/private/tmp/factory-durable-recovery-lint-cache /private/tmp/factory-quality-tools/golangci-lint run --config packs/go/.golangci.yml ./...
0 issues.

GOCACHE=/private/tmp/factory-durable-recovery-go-cache /private/tmp/factory-quality-tools/gosec ./...
Files: 131; Lines: 20892; Nosec: 17; Issues: 0

GOCACHE=/private/tmp/factory-durable-recovery-go-cache /private/tmp/factory-quality-tools/govulncheck ./...
No vulnerabilities found.

GOCACHE=/private/tmp/factory-durable-recovery-go-cache go vet ./...
exit status 0; no output

GOCACHE=/private/tmp/factory-durable-recovery-go-cache go build -o /private/tmp/factory-restoration-reviewed ./cmd/factory
exit status 0; no output
```

The gosec entry in the output block condenses its four summary lines. Independent
correctness/test-quality and security reviewers rechecked both corrections
under the race detector and reported no surviving findings. Latest-head hosted
Linux/macOS CI and the advisory model review are separate publication checks;
these focused local acceptance runs do not claim the entire acceptance suite ran.

### Hosted review follow-up

Copilot identified a same-observation failure boundary: replacing the currently
read asset's ancestor inside a read returning EIO could downgrade the assessment
failure to status 2. The independent spec-writer reproduced it before correction,
alongside successful-read and historical unpinned-observer controls:

```text
FACTORY_AGENT_ROLE=spec-writer GOCACHE=/private/tmp/factory-durable-recovery-go-cache go test -v ./internal/assessment -ginkgo.focus='same-asset parent replacement inside one installed observation' -ginkgo.no-color -ginkgo.succinct
Ran 3 of 142 Specs in 0.072 seconds
FAIL! -- 2 Passed | 1 Failed | 0 Pending | 139 Skipped
FAIL github.com/anoop2811/software-factory-template/internal/assessment 0.644s
```

After the planning-only correction, the same three controls passed with the
race detector. The whole assessment and recovery-command suites also passed:

```text
FACTORY_AGENT_ROLE=spec-writer GOCACHE=/private/tmp/factory-durable-recovery-go-cache go test -race -v ./internal/assessment -ginkgo.focus='same-asset parent replacement inside one installed observation' -ginkgo.no-color -ginkgo.succinct
3/142 selected specs: SUCCESS! 73.596666ms
ok github.com/anoop2811/software-factory-template/internal/assessment 1.416s

FACTORY_AGENT_ROLE=spec-writer GOCACHE=/private/tmp/factory-durable-recovery-go-cache go test -race -v ./internal/assessment ./internal/recoverycmd -ginkgo.no-color -ginkgo.succinct
142/142 assessment specs: SUCCESS! 10.639766625s
26/26 recovery-command specs: SUCCESS! 5.50227575s
```

The successful-read control still refuses with status 2, and the unpinned
observer still classifies changed ancestry as unsafe. The separate GLM advisory
completed and its filesystem, identifier and citation concerns were checked
against the complete helpers; those definitions refuted its conditional claims.
Command fault specifications additionally use the production recovery dispatch
instead of a test-only exported wrapper. Publication qualification records the
latest source checks and hosted results separately from the earlier runs above.

After the wrapper removal, the independent spec-writer reran the command suite:

```text
FACTORY_AGENT_ROLE=spec-writer GOCACHE=/private/tmp/factory-durable-recovery-go-cache go test -race -v ./internal/recoverycmd -ginkgo.no-color -ginkgo.succinct
26/26 specs: SUCCESS! 4.6978745s
ok github.com/anoop2811/software-factory-template/internal/recoverycmd 6.250s
```

The compiled acceptance and whole-source quality commands above were also rerun
on the corrected source. Acceptance passed all 73 selected cases, package
`ok` in 59.455s; lint reported `0 issues.`; gosec reported 131 files, 20887 lines,
17 Nosec and 0 issues; govulncheck reported `No vulnerabilities found.`; vet and
build exited 0. Independent final correctness/test-quality race qualification
passed 20 observation-lifetime and all 26 command cases; security qualification
passed all three same-observation controls. Both reviewers reported zero
surviving findings. Hosted checks must still be read at the published commit.
