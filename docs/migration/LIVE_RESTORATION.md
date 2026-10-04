# Live publication and restoration core

Status: source component; not released or connected to an installation command.
[ADR 0094](../adr/0094-live-publication-restoration.md)
defines the component and its qualification requirements.

The source component replaces one existing known reference file while retaining
the ability to restore its original during the same operation. An existing
integrity-checked recovery set supplies the original bytes. A fresh single-path
adoption confirmation qualifies the current file. Neither input proves that a
factory publication occurred: ownership comes from the actual prepared inode
that this live handle publishes.

## Lifecycle

The assessment package exposes an opaque `Publication` handle through
`BeginPublication`, with explicit `Apply`, `Restore` and `Close` methods. The
request names a recovery set, one catalog path, its adoption confirmation and
replacement bytes. It copies those bytes and preserves the original complete
mode. The initial component supports existing ordinary, single-link catalog
files within the existing 1 MiB bound.

Handle value copies share one private lifecycle, mutex, pinned ownership and
closure state. A copy cannot turn an applied operation into a prepared abort.
Nil and zero-state handles refuse; there is no reconstruction/import API.

Begin retains the exclusive runtime transition guard, checks the whole saved
set, pins the current file and creates durable empty 0600 evidence at
`.factory/runtime-publication.pending`. Apply uses the shared exclusive sibling
preparer. After replacement, the handle retains the actual published inode,
bytes and complete mode. It does not accept an imported ownership receipt.

Restore checks that the owned after-image remains unchanged before preparing
and publishing the original. A later edit, chmod, replacement, deletion, link,
unsafe ancestor or changed backup is a preserved conflict. Identical bytes in
another inode are still a conflict. The saved set, hold, unselected files and
runtime history remain untouched.

Only a checked, durable return to the original permits removal of the exact
owned pending entry. Restore can also abort a prepared operation whose original
has remained unchanged. Close releases resources and the guard; it does not
implicitly restore or remove unresolved evidence.

## Failure boundaries

An error after replacement may mean active state changed. Typed publication
uncertainty must survive joined cleanup errors. An explicit restore may still
use the retained live ownership proof; an automatic rollback is not performed.
A restore whose rename completed but directory sync failed may retry durability
only while its own restored inode and before-image remain unchanged.

Close preserves unresolved pending evidence; abrupt process death loses the
live capability. Both Go and legacy Python shared/fresh exclusive admission
refuse any entry at that path before cooperating budget or loop execution starts.
An unlocked permanent lock, marker age, PID or empty history does not authorize
its deletion.
There is no force-cleanup command or reconstructed live handle in this slice.

A pending-parent sync failure after checked removal is reported without
creating another marker. The active original must already have been durably
restored before that removal is attempted.

## Installation limits

This is a filesystem-only source API. Its lifecycle launches no Git queries,
native probes, scripts, subprocesses, model calls or background jobs. Fixture
setup can create a recovery set before the component begins; creation's Git
orchestration is not part of the live engine.

The future installation caller still owns target qualification, dependency
closure, current-state compatibility, active/uncertain accounting and checkpoint
assessment, explicit unbridged-process quiescence and mandatory activation
checks. Cooperating admission does not establish that older unbridged processes
are absent. Filesystem revalidation inherits the trusted, quiescent namespace
boundary; it is not an atomic compare-and-swap against a hostile same-user writer.

No public restore/rollback/activation flag, installer cutover, script retirement,
retention pruning or release readiness is provided here. The installed shell
dispatcher and the [blocked read-only restoration planner](RECOVERY_RESTORATION_PLAN.md)
retain their existing behavior. A seamless v0.1.6 migration still requires the
complete inventory/backup, durable transaction recovery, public compatibility
integration, activation, retirement, retention and release/pilot qualification.

## Qualification record

The independent spec-writer observed six pending-admission failures against
unchanged production guards before their correction. A separate missing-API
compile failure preceded interface-only unsupported stubs. Three lifecycle
specs then failed at runtime after their real recovery-creation and fresh
consent fixtures succeeded:

```text
go test -v ./acceptance -ginkgo.focus='Live publication pending admission core' -ginkgo.no-color -ginkgo.succinct
Ran 6 of 2388 Specs in 2.931 seconds
FAIL! -- 0 Passed | 6 Failed | 0 Pending | 2382 Skipped
FAIL github.com/anoop2811/software-factory-template/acceptance 3.414s

go test -v ./acceptance -ginkgo.focus='Live publication reversible component core' -ginkgo.no-color -ginkgo.succinct
Ran 3 of 2391 Specs in 0.946 seconds
FAIL! -- 0 Passed | 3 Failed | 0 Pending | 2388 Skipped
FAIL github.com/anoop2811/software-factory-template/acceptance 1.511s
```

Both runtime RED commands used `FACTORY_AGENT_ROLE=spec-writer`, the existing
Go cache and the qualified Python 3.12.14 interpreter. A wider early run included
negative controls passing against unsupported stubs; that is coverage evidence,
not a claim that every new case independently failed before implementation.

Independent review reproduced two additional paired regressions before their
corrections. An actual unsafe prepared descriptor plus real close/EIO returned
refusal status 2 instead of operational status 1; its confirmed-close control
passed. A prepared abort with actual pending unlink followed by parent-sync EIO
made Close claim the absent marker remained; the applied-operation control hid
the inner wording through the typed error. Each pair produced one pass and one
failure. The corrected source reports the close failure and uses neutral
unfinished-operation wording requiring local inspection.

Final security review then reproduced a direct public handle value-copy defect
before correction: copying before apply let Restore return success and remove
pending while the named replacement remained. The single external API control
failed at runtime without disabling vet. Moving lifecycle and resource ownership
behind one private shared state made the same control restore the actual original
and made every alias observe the same closed state. Six additional nil/zero-state
controls were written after that correction; they are not independent RED claims.

The final independent spec-writer race qualification on frozen source used:

```text
FACTORY_AGENT_ROLE=spec-writer FACTORY_CLI_TEST_RACE=1 go test -race -timeout=120s -v ./acceptance -ginkgo.focus='Live publication' -ginkgo.no-color
Ran 85 of 2467 Specs in 38.762 seconds
SUCCESS! -- 85 Passed | 0 Failed | 0 Pending | 2382 Skipped
ok github.com/anoop2811/software-factory-template/acceptance 41.092s

FACTORY_AGENT_ROLE=spec-writer go test -race -timeout=120s -v ./internal/assessment ./internal/filepublish ./internal/transition -ginkgo.no-color
assessment: 161/161 SUCCESS! 10.259s; package ok 11.599s
filepublish: 11/11 SUCCESS! 0.051s; package ok 1.967s
transition: 47/47 SUCCESS! 0.310s; package ok 1.930s
```

These commands ran through `rtk proxy` with the task Go cache; the external
command also used the existing qualified Python 3.12.14 PATH. The internal
lines condense suite/package output. No selected external or internal case
skipped. This change adds 118 cases across six files: 85 external and 33 internal.
Real SIGKILL controls cover both prepared and applied handles, confirm that the
OS flock became available, and still observe Go/Python refusal before help/check
execution. The engine's no-subprocess test starts after production backup creation.

Scoped canonical Go-pack lint over assessment, filepublish, transition and
acceptance reported `0 issues.`; `git diff --check` exited 0. Complete repository
qualification, final independent review and current-head hosted Linux/macOS
checks are separate publication requirements recorded in the pull request.
