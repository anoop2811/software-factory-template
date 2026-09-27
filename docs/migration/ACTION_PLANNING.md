# Go migration target-action planning

[ADR-0080](../adr/0080-go-migration-action-planning.md) extends the read-only
installation assessment into a blocked comparison with a local source tree.

The private compiled request is `FACTORY_BRIDGE_PROTOCOL=1 factory migration
plan ROOT SOURCE`. Its scope remains the same six legacy budget/loop paths.
It lists candidate additions/replacements/retirements, retained files, preserved
customizations, absent paths and conflicts. Retirement intent is compiled for
three known Python files; source omissions do not define an arbitrary delete list.

The planner never treats local metadata or equality to a target as ownership.
Its results always report unproven prior origin, unverified local source,
unqualified recovery and blocked activation. Status 2 with a complete report
means those prerequisites remain unresolved; per-file I/O errors take status 1.
The command performs no writes, downloads, execution or public upgrade dispatch.

## Qualification evidence

Independent compiled RED was observed on 2026-09-26 before production changes:

```text
FACTORY_AGENT_ROLE=spec-writer go test ./acceptance -ginkgo.focus="G4 migration action planning core" -ginkgo.no-color -count=1 -v
Ran 3 of 1519 Specs in 1.665 seconds
FAIL! -- 0 Passed | 3 Failed | 0 Pending | 1516 Skipped
FAIL github.com/anoop2811/software-factory-template/acceptance 2.180s
```

All three requests reached the existing compiled `factory bridge: unsupported
request` refusal.

Final focused qualification combined 46 new planning cases with the 35 original
assessment regressions, instrumenting the actual compiled CLI with the race
detector. Internal coverage combined six new plan faults with the 12 prior
assessment faults:

```text
FACTORY_AGENT_ROLE=spec-writer FACTORY_CLI_TEST_RACE=1 go test -race ./acceptance -ginkgo.focus="G4 migration action planning|G4 installation assessment" -ginkgo.no-color -count=1 -v
Ran 81 of 1562 Specs in 33.784 seconds
SUCCESS! -- 81 Passed | 0 Failed | 0 Pending | 1481 Skipped
ok github.com/anoop2811/software-factory-template/acceptance 35.124s

FACTORY_AGENT_ROLE=spec-writer go test -race ./internal/assessment -count=1 -v
Ran 18 of 18 Specs in 0.022 seconds
SUCCESS! -- 18 Passed | 0 Failed | 0 Pending | 0 Skipped
ok github.com/anoop2811/software-factory-template/internal/assessment 1.549s
```

The controlled planning cases replace either root while the other tree is being
read, inject source open/read errors and cancel during either side's read. They
assert discarded partial results or safe error plans, plus opened-leaf closure.
Independent correctness/security review found no remaining actionable finding;
security separately ran all six new controlled cases under race detection.

Permission-negative cases are conditional when the user/filesystem cannot
enforce their fixtures; this focused local run skipped no selected case. Complete
source-gate and current-head platform CI evidence are recorded in the accompanying
PR. These checks do not establish release authenticity, installation ownership,
runtime activation or live native-client enforcement.

## Progress boundary

Only the target-action half of ADR-0079's second ownership/preview milestone is
in scope. The separate [explicit adoption design](../adr/0081-explicit-legacy-asset-adoption.md)
permits operator-granted authority without asserting historical provenance.
Its qualification and integrated public preview are separate from this original
planner; ordinary `plan` remains blocked and never obtains consent from metadata. This slice cannot close G2 activation/retirement or the complete
G4 lifecycle, and does not turn source comparison into an applicable upgrade.
