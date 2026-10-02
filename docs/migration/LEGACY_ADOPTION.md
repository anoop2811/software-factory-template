# Legacy adoption and transparent cleanup

[ADR-0081](../adr/0081-explicit-legacy-asset-adoption.md) records the approved
ownership boundary. This document describes the source qualification flow;
installed upgrade activation, whole-factory backup and pruning remain pending.
The separate [local recovery writer](RECOVERY_CREATION.md) can preserve an
explicitly confirmed selection from the reviewed reference catalog.

## What an adopter should see

An upgrade preview must list the exact additions, replacements, removals,
compatibility adapters and preserved customizations. For unchanged known legacy
files without reliable installation records, an explicit confirmation grants new
management authority over the selected files. It does not claim historical proof.
The [source-built public preview](PUBLIC_UPGRADE_PREVIEW.md) combines proposal
inspection and confirmation. Installed application remains pending; users should
not have to maintain ownership receipts or hidden migration files.

A changed file invalidates the confirmation. Customized, unknown, missing or
unsafe selected files cannot be adopted through this route. They remain visible
conflicts to resolve; a partial authorization does not enable a complete upgrade.
Neither a saved report nor project configuration can grant consent. Before any
future replacement or removal, the factory must revalidate under migration
exclusion and satisfy target authenticity, compatibility and recovery checks.
Migration validation must use deterministic checks without paid model calls.

## No silent accumulation

The read-only proposal and planning operations create no project files, receipts,
caches or backups. During eventual application, retired implementations leave
active paths; required public script paths remain only as thin compatibility
adapters. Backups are inert private data under the local ignored
`.factory/backups/MIGRATION_ID/` directory, excluded from execution, ordinary
search/context collection, tests and release packaging.

For A -> B, retain recovery for A. After a successful distinct B -> C release
upgrade and its checks, retain recovery for B and prune eligible recovery for A.
A retry, failed upgrade, same-release run or unversioned build does not advance
retention. An edited, held, unsafe or incomplete backup is preserved and listed
with its paths, count, bytes, reason and next action. Failed pruning must report
incomplete cleanup; it cannot silently declare the entire upgrade clean.
This exception handling protects user data, so a strict zero-leftovers guarantee
would be misleading. Normal eligible recovery is bounded automatically; every
exception is visible. See the [canonical recovery requirements](../../specs/001-go-runtime-conversion.md).

## Private source protocol

`FACTORY_BRIDGE_PROTOCOL=1 factory migration propose-adoption ROOT PATH...`
produces a reviewable proposal for one to six explicit catalog paths. A digest
binds the purpose, reference revision, physical root and full observed identities.
The digest identifies that proposal; it is not authentication or a secret.

`FACTORY_BRIDGE_PROTOCOL=1 factory migration plan-adopted ROOT SOURCE DIGEST PATH...`
requires explicit confirmation of the same selection and current observations.
It reports `ownership_basis=explicit_operator_adoption` and authorized paths while
keeping `prior_origin=unproven`. Partial selection leaves whole-scope ownership
blocked. All adopted plans remain blocked on target authentication, runtime
qualification, quiescence and verified recovery; status 2 is expected.

Existing `assess` and `plan` requests retain their original contracts. These
private commands do not activate Go for an installed script-based factory.

## Qualification

Independent compiled RED was observed before production: two proposal requests
failed before implementation, followed by an independently calculated digest
request failing before adopted-plan registration.

```text
FACTORY_AGENT_ROLE=spec-writer go test ./acceptance -ginkgo.focus="G4 explicit legacy adoption core" -ginkgo.no-color -count=1 -v
Ran 2 of 1564 Specs in 1.034 seconds
FAIL! -- 0 Passed | 2 Failed | 0 Pending | 1562 Skipped

FACTORY_AGENT_ROLE=spec-writer go test ./acceptance -ginkgo.focus="G4 explicit legacy adoption supplied digest core" -ginkgo.no-color -count=1 -v
Ran 1 of 1592 Specs in 0.913 seconds
FAIL! -- 0 Passed | 1 Failed | 0 Pending | 1591 Skipped
```

Final focused qualification includes 30 new compiled cases and the 81 original
assessment/planning regressions, with the compiled child itself race-instrumented:

```text
FACTORY_AGENT_ROLE=spec-writer FACTORY_CLI_TEST_RACE=1 go test -race ./acceptance -ginkgo.focus="G4 explicit legacy adoption|G4 migration action planning|G4 installation assessment" -ginkgo.no-color -count=1 -v
Ran 111 of 1592 Specs in 46.079 seconds
SUCCESS! -- 111 Passed | 0 Failed | 0 Pending | 1481 Skipped
ok github.com/anoop2811/software-factory-template/acceptance 47.554s

FACTORY_AGENT_ROLE=spec-writer go test -race ./internal/assessment ./internal/bridge -count=1 -v
SUCCESS! -- 26 Passed | 0 Failed | 0 Pending | 0 Skipped
ok github.com/anoop2811/software-factory-template/internal/assessment 1.511s
SUCCESS! -- 4 Passed | 0 Failed | 0 Pending | 0 Skipped
ok github.com/anoop2811/software-factory-template/internal/bridge 1.697s
```

Controlled cases cover same-byte inode replacement during planning and before
final reobservation, replacement of either root, selected read failure and
cancellation with handle closure. A fixed digest vector and field-coverage guard
cover the canonical payload; a final guard refinement was separately rerun:

```text
FACTORY_AGENT_ROLE=spec-writer go test -race ./internal/assessment -ginkgo.focus="Legacy adoption canonical digest" -ginkgo.no-color -count=1 -v
SUCCESS! -- 2 Passed | 0 Failed | 0 Pending | 24 Skipped
ok github.com/anoop2811/software-factory-template/internal/assessment 1.373s
```

Independent correctness and security review reported no remaining actionable
finding. Full source-gate and current-head platform CI evidence belong in the PR.
These checks do not establish installed upgrade, recovery or retention behavior;
those lifecycle obligations remain open. Permission-negative fixtures are
conditional on the host's ability to enforce permissions; this local focused run
skipped no selected case.
