# Go installation reference assessment

[ADR-0079](../adr/0079-go-installation-reference-assessment.md) defines the first
read-only prerequisite for ownership-aware budget/loop migration.

The private compiled request is `FACTORY_BRIDGE_PROTOCOL=1 factory migration
assess ROOT`. It compares six fixed legacy budget/loop files to immutable
reference bytes, types and modes. Matching is evidence about content, not the
origin or ownership of an installation. Every result explicitly denies ownership
authorization and activation readiness.

The operation does not execute a candidate, model, Git command or installed
script. It creates no plan file, lock, temporary file or backup. It does not read
local version/ownership claims as authority. Public upgrade dispatch, target
action planning, recovery and cleanup remain pending.

## Qualification evidence

Independent compiled core RED was observed on 2026-09-26 before implementation:

```text
FACTORY_AGENT_ROLE=spec-writer go test ./acceptance -ginkgo.focus="G4 installation assessment core" -ginkgo.no-color -count=1 -v
Ran 3 of 1484 Specs in 1.062 seconds
FAIL! -- 0 Passed | 3 Failed | 0 Pending | 1481 Skipped
FAIL github.com/anoop2811/software-factory-template/acceptance 1.394s
```

All three requests reached the existing compiled refusal, `factory bridge:
unsupported request`.

Focused compiled qualification subsequently ran with actual CLI race
instrumentation, including immutable catalog comparison, exact schema, bounded
files, special modes, links/devices, literal root operands, forged metadata,
permission failures and unchanged fixture contents/modes:

```text
FACTORY_AGENT_ROLE=spec-writer FACTORY_CLI_TEST_RACE=1 go test -race ./acceptance -ginkgo.focus="G4 installation assessment" -ginkgo.no-color -count=1 -v
Ran 35 of 1516 Specs in 12.442 seconds
SUCCESS! -- 35 Passed | 0 Failed | 0 Pending | 1481 Skipped
ok github.com/anoop2811/software-factory-template/acceptance 13.902s

FACTORY_AGENT_ROLE=spec-writer go test -race ./internal/assessment -count=1 -v
Ran 12 of 12 Specs in 0.012 seconds
SUCCESS! -- 12 Passed | 0 Failed | 0 Pending | 0 Skipped
ok github.com/anoop2811/software-factory-template/internal/assessment 1.369s
```

The controlled filesystem cases mutate files only from test collaborators after
production opens the descriptors: content growth, modes/timestamps, leaf and
ancestor replacement, added hard links, and replacement of the project root.
They also exercise open/read failures, mid-read cancellation and handle closure.
Permission cases are conditional when the executing user/filesystem cannot
enforce their negative fixture; the local run above skipped no selected case.

Independent correctness and security reviews found no remaining actionable
issue. Security independently ran all 12 controlled cases under race detection
and the 14-case compiled classification subset. Complete source-gate and
current-head platform CI evidence are recorded in the accompanying PR rather
than inferred from these focused results. No live native model call is involved.

## Progress boundary

This slice is one of three milestones in the existing G4 ownership/preview
package. The other two are trusted prior-origin/target-action planning and the
integrated public dry-run boundary. No G2 activation/retirement milestone or G4
release stage is completed by reference assessment alone.
