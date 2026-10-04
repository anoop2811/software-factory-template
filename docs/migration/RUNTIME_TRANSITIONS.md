# Runtime transition guard

Decision 92 and [ADR 0093](../adr/0093-runtime-transition-guard.md) introduce
cooperation between the source-built Go controllers and the current legacy
budget/loop scripts. This is a prerequisite for safe installed migration, not
an installer or a completed v0.1.6 cutover.

## What participates

An eligible explicit budget run enters before native capability preflight and
holds its guard through admission, execution, cleanup and terminal accounting.
An eligible manual or bounded loop enters before its execution snapshots and
holds the guard through terminal checkpoint publication. A bounded loop's
nested budget calls use the same shared exclusion protocol. Admission and
spending limits still come from the existing budget configuration.

Read-only plans, status and reports, disabled runs and initially blocked runs
do not create transition storage or acquire a guard. No model, background
service or network request is added by the protocol.

## Local control objects

The permanent `.factory/runtime-transition.lock` is an empty private regular
file. Cooperating work holds a shared operating-system flock on this same
inode. The future installation transaction must hold an exclusive flock;
contention refuses before conflicting work starts. Neither side removes or
replaces the lock file.

Before executing a child, each participant durably creates its own private,
empty marker under `.factory/runtime-activity/`. Normal confirmed completion
removes only that marker and syncs the directory. Parent death or uncertain
process ownership/publication retains evidence. These markers contain no
prompts, responses, credentials or PID-based expiry authority.

Control storage rejects unsafe types, links, ownership, permissions and
identity changes. Existing unsafe objects are preserved for inspection.
Normal successful work leaves an empty activity directory and the permanent
lock. Runtime history remains in the existing budget/checkpoint records.

Initial admission refusals remain read-only. A later conflict discovered by an
execution snapshot, such as an unmerged Git index, can leave these private guard
controls and conservative activity evidence. It still starts no check/model and
publishes no budget or checkpoint history. That evidence is retained for explicit
inspection rather than silently treated as harmless because the command stopped.

## Interrupted and uncertain work

An unlocked inode does not prove that a descendant exited. Any remaining
activity entry blocks the exclusive component, including an unrecognized or
malformed entry. It does not probe PIDs, expire markers by age or delete them.
Keep the evidence and inspect the corresponding process and accounting state.
There is no automatic activity-marker recovery command in this slice.

Read-only diagnostics remain available. The guard does not reset reservations,
budget consumption, checkpoints, fingerprints or Git state. Existing execution
admission rules remain responsible for active or uncertain accounting records.

## What this does not establish

A process launched from v0.1.6 or another installation without the guard does
not participate simply because the files on disk were upgraded. Successful
exclusive acquisition applies only to cooperating participants; it grants no
historical ownership or permission to replace active legacy files. A later
transaction must separately establish explicit quiescence for unbridged work,
state compatibility, backup integrity and exact forward/reverse ownership.

The public installed `factory` entrypoint still uses scripts. Authenticated
binary installation, activation, complete legacy backup, controlled restoration,
retirement, retention and the adopter pilot remain pending. No release or
seamless-upgrade claim follows from this component.

## Qualification status

The contract was committed before implementation. The independent spec-writer
ran the initial compiled Go and actual legacy entrypoint controls against the
unchanged production code:

```text
FACTORY_AGENT_ROLE=spec-writer GOCACHE=/private/tmp/factory-durable-recovery-go-cache PATH="/var/folders/83/yj7qqyt551xbbpvm54tqkcbw0000gn/T/factory-pr113-python-_gp80gpr/venv/bin:$PATH" go test -v ./acceptance -ginkgo.focus='Runtime transition exclusion core' -ginkgo.no-color -ginkgo.succinct
Ran 4 of 2315 Specs in 2.680 seconds
FAIL! -- 0 Passed | 4 Failed | 0 Pending | 2311 Skipped
FAIL github.com/anoop2811/software-factory-template/acceptance 3.367s
```

Both budget controllers started the native-help sentinel despite real exclusive
exclusion; both manual loop controllers started the check sentinel and completed
with status 0. These four observed failures authorized production work. Fixture
role permissions were valid, and the pre-existing state parent used mode 0755.

Further independent regressions observed a successful legacy help leader and
successful Go/legacy snapshot leaders leaving live same-group descendants after
closing their output pipes. These controls failed before the shared owned-probe
corrections. Actual-module compound fault controls also failed before correction
when release hid a typed process error or a secondary cleanup diagnostic. Their
corrections preserve the original ownership/PID or I/O errno while reporting
cleanup failure through the direct error channel. No process liveness is inferred
from a successful leader or terminal record alone.

The final focused public qualification ran on 2026-10-04 UTC with both the
compiled child CLI and its outer test process instrumented for races:

```text
FACTORY_AGENT_ROLE=spec-writer FACTORY_CLI_TEST_RACE=1 GOCACHE=/private/tmp/factory-durable-recovery-go-cache PATH="/var/folders/83/yj7qqyt551xbbpvm54tqkcbw0000gn/T/factory-pr113-python-_gp80gpr/venv/bin:$PATH" go test -race -v ./acceptance -ginkgo.focus="Runtime transition" -ginkgo.no-color -ginkgo.succinct
--- PASS: TestAcceptance (42.40s)
PASS
ok github.com/anoop2811/software-factory-template/acceptance 43.924s
```

All 71 selected scenarios passed. These exercise actual Go/legacy entrypoints,
shared/exclusive flock coordination, paused pre-admission help, abrupt controller
death, helper descendants, nested bounded owners, normal cleanup, read-only and
unsafe-storage controls, compound errors, real syscall fault collaborators and
actual source init/upgrade delivery. Fourteen Python storage controls and 41 Go
collaborator controls were added after implementation; do not interpret their
passing results as independent pre-implementation RED for every fault.

The separate Go collaborator race command and its observed output were:

```text
FACTORY_AGENT_ROLE=spec-writer GOCACHE=/private/tmp/factory-durable-recovery-go-cache go test -race -v ./internal/transition -ginkgo.no-color -ginkgo.succinct
--- PASS: TestTransition (0.23s)
PASS
ok github.com/anoop2811/software-factory-template/internal/transition 1.597s
```

All 41 selected controls passed. Injected sync, read, open, close and unlink
failures exercise real descriptors and storage. Identity replacement preserves
foreign evidence; incomplete ownership retains its marker; failed post-unlink
directory sync reports failure without recreating a marker. These are focused
results. Complete repository qualification remains pending until the full source
gate and hosted Linux/macOS runs have completed green. Consult the PR's check
records for their results.
These observations qualify this source component, not installation activation,
unbridged-process quiescence or a complete v0.1.6 release migration.

Hosted review also identified two Go cleanup defects. Independent real-process
controls distinguished a reaped leader and acknowledged signal from actual group
absence; a separate real checkpoint publication followed by guard-release
failure exposed loss of `PublicationError` and `MayHaveCommitted`. Both pairs
failed before their respective corrections. The shared supervisor now requires
explicit group absence within its original cleanup deadline, and loop cleanup
preserves the primary error chain alongside a safe release diagnostic. See
docs/adr/0093-runtime-transition-guard.md:195 and
docs/adr/0093-runtime-transition-guard.md:205.

Observed on the corrected source, 2026-10-04 UTC, using the qualified Python PATH
and task Go cache:

```text
FACTORY_AGENT_ROLE=spec-writer go test -race -v ./internal/native ./internal/loop -ginkgo.focus="Native process group completion proof|Loop transition compound publication evidence" -ginkgo.no-color -ginkgo.succinct
native: 2/6 selected; SUCCESS! 5.152255209s; package 6.399s
loop: 2/82 selected; SUCCESS! 242.837459ms; package 1.704s
FACTORY_AGENT_ROLE=spec-writer FACTORY_CLI_TEST_RACE=1 go test -race -v ./acceptance -ginkgo.focus="Runtime transition" -ginkgo.no-color -ginkgo.succinct
71/2382 selected; SUCCESS! 36.067746s; package 37.679s
```

Inspection errors remain unconfirmed while bounded polling continues. Only an
explicit `ESRCH` before expiry qualifies absence; neither signal acknowledgement
nor `EPERM` grants cleanup authority. These focused observations do not replace
the full repository and hosted qualification required above.
