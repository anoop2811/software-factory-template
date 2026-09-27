# Public Go upgrade preview

[ADR-0082](../adr/0082-go-public-upgrade-preview.md) defines the read-only preview
in a locally built Go binary. Installed migration application remains pending.

Run from the installation root:

```sh
/path/to/source-built/factory upgrade --dry-run --source /path/to/local/target
/path/to/source-built/factory upgrade --dry-run --source /path/to/local/target --json
```

This previews six known legacy budget/loop paths. It shows candidate actions,
preserved customizations, conflicts and unresolved prerequisites. It does not
assess the rest of the factory, authenticate the local target or declare an
applicable upgrade. A complete blocked preview exits 2; I/O failures exit 1.

The current directory is the installation root. Preview does not invoke Git,
read factory configuration, discover parent repositories or use the binary's
location as the target. Relative source paths resolve from the caller's current
directory. Source paths retain the confined reader's rules: no .. components
or symlink ancestors; use a physical directory path. This differs from the
existing legacy upgrader's Git-root discovery.
No script, check, model, download, cache, receipt or backup is created or run.

Do not remove --dry-run expecting to apply this plan. Ordinary upgrade still
selects the existing legacy script and is outside this preview's scope. Installed
Go activation and transactional apply remain separate pending work. The tracked
shell factory and direct installed scripts are unchanged by this source feature.

## Explicit adoption

Add one --adopt-path option per unchanged known legacy path to request a proposal:

```sh
/path/to/source-built/factory upgrade --dry-run --source /path/to/local/target --adopt-path scripts/factory-budget.sh
```

The report includes the selected paths and a digest bound to the observed
installation and file identities. Review that scope, then rerun the same command
and selection with --confirm-adoption followed by the reported digest. Changes
to selected files invalidate confirmation. A partial selection authorizes only
its named paths and leaves complete ownership blocked; an unconfirmed proposal
authorizes nothing. All remaining activation/recovery blockers stay visible even
when all six unchanged assets have explicit confirmation.

The [adoption contract](LEGACY_ADOPTION.md) distinguishes new management authority
from historical provenance. Saving a report never grants future authority.
Application will need revalidation under exclusion and verified recovery first.

## Safe argument handling

A standalone --dry-run token or --dry-run= prefix reserves the preview route
even in an invalid request. Invalid requests cannot fall back to the mutating
script. Use one bare --dry-run, one nonempty --source, and optional bare --json.
Adoption paths may repeat as options with distinct catalog values; other duplicate
options, --ref, unknown arguments, help flags and attached boolean values refuse.

Use --source=--name or ./--name for literal dash-leading source paths. The
ambiguous spelling --source --dry-run refuses as preview intent. Without a
preview marker, --source=--dry-run remains a literal legacy-script invocation.

## Recovery and cleanup boundary

Preview creates no local migration files and does not inspect backup folders.
It reports recovery as not assessed instead of inventing an empty inventory.
The [canonical recovery requirements](../../specs/001-go-runtime-conversion.md)
still require private ignored backups, removal of retired active implementations,
and pruning of eligible older sets after a successful later release. Edited,
held and unsafe exceptions must remain visible. This source preview does not
implement or claim that lifecycle.

## Qualification evidence

Independent compiled RED preceded implementation:

```text
FACTORY_AGENT_ROLE=spec-writer go test ./acceptance -ginkgo.focus="G4 public upgrade preview core" -ginkgo.no-color -count=1 -v
Ran 4 of 1596 Specs in 2.079 seconds
FAIL! -- 0 Passed | 4 Failed | 0 Pending | 1592 Skipped
```

Three additional safe-reason cases and one cancellation diagnostic case were
observed failing before the actionable-error correction. Final qualification
combined 64 new public-preview cases, 111 existing assessment/planning/adoption
cases and all 41 command-boundary cases, instrumenting the compiled child itself:

```text
FACTORY_AGENT_ROLE=spec-writer FACTORY_CLI_TEST_RACE=1 go test -race ./acceptance -ginkgo.focus="G4 public upgrade preview|G4 explicit legacy adoption|G4 migration action planning|G4 installation assessment|The developer-built Cobra command boundary" -ginkgo.no-color -count=1 -v
Ran 216 of 1656 Specs in 115.520 seconds
SUCCESS! -- 216 Passed | 0 Failed | 0 Pending | 1440 Skipped
ok github.com/anoop2811/software-factory-template/acceptance 117.043s

FACTORY_AGENT_ROLE=spec-writer go test -race ./internal/assessment ./internal/upgradecmd -ginkgo.no-color -count=1 -v
SUCCESS! -- 34 Passed | 0 Failed | 0 Pending | 0 Skipped
ok github.com/anoop2811/software-factory-template/internal/assessment 1.602s
SUCCESS! -- 8 Passed | 0 Failed | 0 Pending | 0 Skipped
ok github.com/anoop2811/software-factory-template/internal/upgradecmd 1.675s
```

Controlled cases qualify unconfirmed/confirmed selected observation changes,
root replacement, read failures, context cancellation, deadline refusal, descriptor
closure and text/JSON write/short-write/flush errors. Cancellation was exercised
through parent contexts and controlled reads; no compiled preview SIGINT/SIGTERM
timing probe was performed. The scoped signal registration received source review.
Permission-negative fixtures are conditional on host enforcement; this local
focused run skipped no selected case.

Independent correctness/security review reported no remaining actionable finding.
Full source-gate and current-head Linux/macOS CI evidence belong in the PR. This
qualification closes source preview coverage for the six-path package only; it
does not establish installed activation, complete factory coverage or cleanup.
