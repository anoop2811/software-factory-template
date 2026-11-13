# ADR 0148: transact existing-project CI migration with installation recovery

Status: implemented; final-head qualification and human review pending.
Decision: 164. Continues R4 after merged PR #178.

## Boundary

PR #178 merged as `24712604cbe98dc2df087b73910141defb68f745` on
2026-11-11 UTC. Its informational CI report establishes a finite ownership
projection but does not apply workflow changes. Compose that projection with
the existing whole-installation transaction, including consent, ignored backup,
publication, interrupted recovery, explicit rollback and later-release retention.
Keep this one functional boundary; privileged review bootstrap and integrated
hosted adopter rehearsal remain separate release obligations.

Do not add commands, flags, dependencies or paid operations. Keep the runtime
role sets, fresh-init Selection and installed descriptor semantics unchanged.
Project workflows are transaction participants, not runtime custody assets.
Editing an application workflow must not prevent ordinary factory commands or
CI preparation from admitting an otherwise healthy runtime.

## Authenticated entry and consent

The ordinary and explicit candidate release owner supplies its acquired target
release identity explicitly. A source archive's metadata, `Candidate`, a local
label or a serialized record alone does not confer authentication. Local-source
and unavailable/ineligible-release flows retain their existing CI preservation
boundary. Existing archive recovery must nevertheless understand the new records:
the emitted `upgrade --installation` route is a required recovery path.

For eligible authenticated forward upgrades, compute CI changes inside the same
shared plan and read-only/exclusive ownership lifetime as runtime changes, before
the installation digest and outer retention binding. Include the exact selected
workflow observations, relevant configuration observation, release identities and
bounded projection inputs in consent. Reconstruct during apply; never consume a
previous JSON report as authority. Changed workflow bytes, mode, identity, parents
or relevant settings invalidate the old confirmation before active mutation.

The public `ci_workflow` report now describes this transaction and sets
`apply_supported: true` only for the eligible integrated route. Its prose must
explain that confirmation includes the listed workflow changes. Preserve explicit
unavailability on local routes. Old informational digests are not approvals for
the expanded transaction. Operational read, cancellation and output failures
remain errors, distinct from preservation conflicts.

## Finite ownership and conflict policy

The component contains exactly `.github/workflows/ci.yml`, `go-pack.yml`,
`typescript-pack.yml`, `java-pack.yml` and `factory.yml`. Reuse ADR 0147's exact
closed copied/generated ownership projection, current application input checks,
historical override domain and mode checks. Do not invent defaults or recognize
ownership from a header. Proposed target bytes use the existing pure renderer
and the acquired target's exact selectors.

Missing copied paths remain absent. Exact owned copied paths retire; an absent
factory workflow is created; an exact supported generated workflow is refreshed.
Customized, unknown, unselected-pack, unsafe, oversized or mode-changed occupants
remain preserved conflicts. Any unresolved selected-path conflict blocks the
combined automatic upgrade before publication. Do not add a new job while leaving
an incompatible selected old job and call that a completed migration. Workflows
outside these five paths remain outside observation and mutation.

## Durable component and compatibility

Represent CI as a bounded, versioned optional transaction component. Centralize
the complete runtime-plus-CI transaction inventory; do not relax existing runtime
role checks into arbitrary allowed-path checks. A CI-bearing record must contain
the complete finite component with no duplicate, missing or extra paths. Its
journal and before-image manifest must agree. Old records without the component
retain their existing exact generation rules. Older readers may conservatively
refuse unfamiliar records; retained new recovery tooling must remain usable.

Persist only allowlisted projection inputs needed to reconstruct both old and
target workflow bytes, not the complete configuration or credentials. They are
descriptive evidence: validate bounds/grammar and reconstruct from freshly
qualified source/predecessor images before trusting any recorded action or saved
byte. Later configuration edits must not make intact historical workflow backups
unrecoverable or permanently unprunable. Do not execute saved scripts, workflows,
or historical renderer code.

Before changing a source that participates in runtime-generation comparison,
freeze the merged runtime projection from immutable
`24712604cbe98dc2df087b73910141defb68f745`. Its unchanged runtime role set and
descriptor remain distinct from the optional CI transaction component. Preserve
all already supported frozen generations. An authenticated but unknown historical
projection is still unsupported; do not broaden generation matching heuristically.

## Apply, interruption and reverse

Reuse the existing exclusion, quiescence, candidate qualification and pending
barrier. Journal the complete intent before mutation. Save exact workflow before
bytes through the same durable private backup machinery at inert mode 0600 under
the existing ignored `.factory/backups/<id>/assets/` tree. Do not create another
backup directory or retention clock. Reuse checked no-follow publication and its
prepared/observed identities for create, replace and retire operations.

Check the complete CI result before successful activation and pending cleanup,
separately from runtime descriptor health. Existing files changed during the
operation must remain conflicts. A failed check cannot report CI migration done.
Retain honest recovery instructions and independently usable candidate inputs
when interruption or publication uncertainty leaves a pending transaction.

Recovery and rollback reconstruct CI before/after projections from fresh qualified
images and the validated recorded context, then use the existing row identity,
phase and ownership rules. Cover preparation-only abandonment, forward and reverse
interruption, completed cleanup and ordinary completed rollback. Missing/tampered
context, manifest, journal or saved bytes fail closed without rewriting later user
edits. A legacy rollback restores the old workflow bytes as inert project files;
it does not execute those restored workflows as part of recovery.

## Retention and absence of cruft

Extend exact backup scanning and independent retention qualification to the finite
CI component. Prove both historical and target bytes from fresh qualified images;
the receipt or hash alone never authorizes deletion. Keep the immediately preceding
usable installation and existing held/edited/unknown exceptions. A later distinct
successful forward release may prune eligible older workflow backups along with
their runtime recovery set, with accurate file/byte totals. Same-release retries,
rollback and local builds do not advance retention. Valid CI files must not become
unclassified leftovers merely because this reader still recognizes runtime rows
alone.

## Independent outside-in qualification

Observe Ginkgo/Gomega RED before implementation in complete private checkouts.
Preserve the operator's ignored binary and unrelated worktree files. Cover:

- Actual authenticated legacy and Go-generated upgrades, exact retired/replaced
  bytes, ignored inert backups and completed rollback through public commands.
- Customized/unsafe known paths, untouched unrelated workflows and changed
  configuration/workflow observations between preview and confirmation.
- Real interruption after workflow publication, with fresh forward and reverse
  recovery using the emitted installation route; later edits remain preserved.
- Preparation abandonment, completed cleanup, malformed/partial component data,
  forged actions and changed saved-byte evidence.
- Later-release pruning of an eligible workflow-bearing recovery set while a
  usable immediate predecessor remains; controls for held/edited/same-release sets.
- Local/ineligible preservation, original record compatibility, frozen predecessor
  reconstruction and runtime admission independence from project CI.

Evolve the old informational-only preview assertions to this explicitly changed
contract. Keep exact discovery, disjoint scheduling and failure propagation
controls. The measured qualification below requires a separate `ci-migration`
partition for the fifteen transaction criteria; the six existing initialized-CI
criteria retain their original partition. Both use one worker and the existing
25-minute allowance. Do not increase process counts or timeouts to accommodate
unrelated performance work.
Run focused race qualification, affected recovery/retention/compatibility checks,
configured lint/vet and review-diamond verification before publication. Hosted
checks and adversarial findings belong to the final PR head. Passing this boundary
does not by itself complete R4, the adopter pilot or stable release qualification.

Qualification fixture refinement: keep the default-init fixture's selected-only
initializer message witness for initialization tests. Upgrade fixtures must leave
the initializer source unchanged so its exact historical recipe can qualify;
their separate selected-only planner and transaction-completion witnesses still
prove binary ownership against the earlier-built invoker. Make that fixture
choice explicit, assert the initializer witness is absent on the upgrade branch,
and remove no existing behavioral criterion or selected-planner/apply assertion.

## Component representation

The initial authenticated owner supplies `Request.CIRelease` separately from
staging metadata. An integrated proposal exposes `ci_workflow` directly; the
retention envelope keeps its existing embedding. Journal and before-image manifest
store the same optional `ci_workflow` context with `schema_version: 1`, `release`,
`before`, `current` and `historical`. Release identifies the exact target; before
identifies the Go predecessor when one exists. Current and historical maps contain
only the enumerated workflow projection inputs and retain the distinction between
current application setup and historical overrides. No persisted context expands
the five-path inventory. Unsupported context versions or keys refuse safely.

The observed current configuration image belongs to forward-plan consent and
rechecks, not a requirement that future live configuration equal its old value.
Reconstruction, recovery and retention validate the recorded semantic inputs and
fresh independent source images. They never use a stored configuration identity
to reject a legitimate later application configuration edit.

Evidence limit: inconsistent recognizable CI context or inventory must refuse.
Completely rewriting both untrusted records into an otherwise valid historical
runtime-only shape cannot prove the erased optional intent. A fresh recovery
proposal then exposes only runtime actions and requires new consent; it grants
no CI write or deletion authority and makes no CI-completion claim. Any residual
CI backup files remain preserved by receipt/inventory checks. Do not invent
authenticated local history or reject legitimate runtime-only records to conceal
that limit. Deliberately erased history may require manual CI reconciliation.

## Local qualification receipts

All commands in this section ran in private macOS arm64 checkouts, using the
existing Go 1.27.3 toolchain, `GOTOOLCHAIN=local`,
`GORACE=atexit_sleep_ms=0` and explicit `FACTORY_AGENT_ROLE=spec-writer`.
No primary-checkout binary was built or executed. Local evidence remains distinct
from final-head hosted qualification and published-release platform evidence.

Before implementation, the independent authenticated legacy migration criterion
failed because `apply_supported` was false. Decision 164 records its exact command
and output. The later initial broad diagnostic was stopped before its local
deadline: nine criteria passed, two test invocations used unsupported command
arguments, one was interrupted and two had not run. The evaluator corrected the
invocations to the documented `metrics --json` and standalone keep-backup routes;
that partial run is not whole-group qualification.

Review regression RED used:

```text
go test -race -count=1 -v ./internal/releasecmd ./internal/installation \
  -ginkgo.focus='CI migration confirmation presentation|only runtime recipe difference' \
  -ginkgo.fail-on-empty -ginkgo.no-color
```

The integrated presentation failed while the unavailable-route control passed.
The unchanged predecessor control passed, but changing only `recipe_v4.go`
returned status 0 instead of the required refusal. After the production
corrections, the same command with focus
`CI migration confirmation presentation|Historical installation recipe integrity`
reported `2 Passed | 0 Failed` and `6 Passed | 0 Failed`. The immutable historical
role oracle was also corrected to use the predecessor's actual closed retirement
transformation; the earlier oracle setup failure is retained in the local record,
not described as a production defect.

Final internal command:

```text
go test -race -count=1 -v ./internal/releasecmd ./internal/installation
```

Observed output: `Ran 2 of 2 Specs in 0.001 seconds`,
`SUCCESS! -- 2 Passed | 0 Failed | 0 Pending | 0 Skipped`, and
`Ran 83 of 83 Specs in 19.054 seconds`,
`SUCCESS! -- 83 Passed | 0 Failed | 0 Pending | 0 Skipped`.

Configured `golangci-lint run --config packs/go/.golangci.yml ./...` reported
`0 issues.`; `go vet ./...` exited 0 with no output. The configured citation hook
reported `citation-lint: no citation_prefix configured — skipping`; it supplies
no mechanical citation-validation evidence. Reviewers checked changed citations
against actual source lines.

The independent review-diamond found no surviving source blocker after verified
presentation and generation-comparison corrections. Tamper criteria now include
healthy recovery controls before and after each corruption. Full public-group,
ordinary-upgrade and real scheduling results remain pending here.

The first configured combined initialized-CI run exhausted its existing allowance:
`Ran 18 of 3365 Specs in 1488.547 seconds` and
`FAIL! - Suite Timeout Elapsed -- 13 Passed | 5 Failed | 0 Pending | 3347 Skipped`;
the runner reported `25m2.684500334s`. The fifteen transaction criteria consumed
1234.779 seconds, and the two preview criteria consumed 123.207 seconds. The first
existing initialization criterion then exhausted the suite allowance; the other
three did not run. Four transaction assertions incorrectly applied successful-init
acquisition cleanup or standalone hold-output expectations to recovery/retention.
Their corrected assertions preserve the actual acquisition ownership and held-set
contracts, including exact saved bytes and healthy controls. This failed run
does not qualify the full group.

Split those fifteen transaction criteria into the distinct `ci-migration` group
instead of extending the timeout or repeating initialization less rigorously.
Update the Linux/macOS matrix, independent exact census, disjointness controls,
one-worker execution and failure propagation together. The total census remains
3365; no criterion is removed or renamed to escape a selection.

The configured default-upgrade run reported `Ran 14 of 3365 Specs in 592.108
seconds` and `SUCCESS! -- 14 Passed | 0 Failed | 0 Pending | 3351 Skipped`.
Its later vulnerability scan failed on `GO-2026-6859` in Go 1.27.3, with Go 1.27.4
as the correction. Thus the behavioral group passed, but the complete make target
did not. The separately scoped compiler-security prerequisite must preserve
historical bundle compatibility; disabling the scan is not a resolution.

The corrected preparation, held-retention and forward-recovery criteria passed
in the focused four-case rerun. The remaining reverse test first observed active
reverse publication, then signalled the same owner; its diagnostic captured a
normal transition from `applying` to `checking` during the 11 ms before SIGSTOP.
Keep the initial `applying` witness. The stopped witness may be `applying`,
`checking` or `metadata_prepared`, but never `committed`: direction, reversed
workflow row, exact original bytes and pending marker remain mandatory, followed
by SIGKILL, reap and an unchanged journal. The independent reviewer confirmed
these are unfinished transaction states, not a completed-operation shortcut.

The subsequent reverse-only race command was:

```text
go test -race -count=1 -v -timeout=10m ./acceptance \
  -ginkgo.focus='initialized CI migration transaction.*reverse restoration' \
  -ginkgo.fail-on-empty -ginkgo.no-color \
  -ginkgo.json-report=../.factory/ci-transaction-evidence/reverse-final.json
```

Observed: `Ran 1 of 3365 Specs in 354.530 seconds` and
`SUCCESS! -- 1 Passed | 0 Failed | 0 Pending | 3364 Skipped`. Fresh consent,
completion through the emitted archive arguments, replay and exact retained-input
preservation all executed. This corrects the observer's phase expectation; no
production recovery change was required.

The partition regression first failed because the old initialized group still
selected migration identities. After the split,
`go test -race -count=1 -v -timeout=15m ./acceptance
-ginkgo.focus='Go source qualification (scheduling|worker execution)'
-ginkgo.fail-on-empty -ginkgo.no-color` reported
`Ran 11 of 3365 Specs in 161.393 seconds` and
`SUCCESS! -- 11 Passed | 0 Failed | 0 Pending | 3354 Skipped`. This includes
actual discovery of all 3365 identities, the exhaustive disjoint partition union,
one/two-worker controls and failure propagation. The separate workflow-matrix
criterion used `go test -race -count=1 -v -timeout=5m ./acceptance
-ginkgo.focus='Go source qualification workflow partition' -ginkgo.fail-on-empty
-ginkgo.no-color -ginkgo.json-report=../.factory/ci-transaction-evidence/partition-workflow-green.json`
and reported `Ran 1 of 3365 Specs in 45.698 seconds` with `1 Passed | 0 Failed`.
Full split-group timing and final-head hosted checks remain separate pending
qualification; source implementation is not a stable release.

## Requalification after the compiler-security prerequisite

PR #179 merged on 2026-11-12 UTC as
`64cd29f9e29a3f5bc3340b29877f41a2432d293f`, with all 34 hosted checks passing.
Rebase this boundary onto that source while retaining the existing frozen runtime
recipe and every migration criterion. The official
[Go release history](https://go.dev/doc/devel/release), fetched 2026-11-12 UTC,
still lists Go 1.27.4 (released 2026-11-10) as the current security patch.

Current builds use Go 1.27.4; immutable historical fixtures retain their actual
Go 1.27.3 compiler through the shared helper from ADR 0149. The CI-migration
partition needs that historical setup alongside the historical-recipe partition.
Capture its absolute path before selecting the current compiler, preserving
the existing workflow-step ordering and explicit environment isolation.

Reconcile the actual discovery census to 3459: 3444 merged criteria plus the
fifteen transaction criteria. Keep the new group disjoint, one-worker, and within
its existing 25-minute allowance. Requalify scheduling, workflow wiring,
installation/release-owner internals and the complete public transaction group.
Previous receipts remain historical evidence; record the current-source results
separately before claiming this integration is qualified.

## Historical workflow mode qualification

The first hosted historical-recipe job on PR #180 head `9b962a3` passed eighteen
of twenty criteria. Its two refusals exposed a real predecessor-mode gap:
the immutable copied Go initializer retained mode 0600 from its private staged
source assets, while CI ownership recognition assumed 0644 for every predecessor.
The genuine installation and its clone both retain that mode; changing the test
fixture to 0644 would conceal the migration defect.

Qualify the expected historical mode together with its exact frozen initializer
recipe and projected bytes. Authenticated copied Go initializers use 0600;
qualified Bash copies and generated workflows use 0644. Carry that evidence
through preview, transaction reconstruction, rollback and retention. Do not accept
both modes indiscriminately, derive ownership from the live occupant, or relax
unknown-recipe and mode-change conflicts. New generated target workflows remain
0644 and saved backup files remain inert 0600.

Independently reproduce the genuine copied-Go refusal, then require successful
preview/application/reverse restoration with the original exact mode. Retain
negative controls for changed modes and for every other initializer family.

The historical-cache retention criterion also intentionally pairs the current
consumer with an older target source recipe lacking the new CI renderer. Keep
target-recipe authentication strict. Exercise that historical retention scenario
through the existing authenticated candidate owner, retaining independently
qualified images, release-based retention, the corrupted alternate proof and all
no-deletion assertions. Use fixture-local abbreviated semantic versions
`v9.7`, `v9.8`, and `v9.9`: acquisition authenticates those exact identities and
retention orders them, while CI explicitly reports its existing noncanonical-tag
boundary. Assert that unavailable-CI result. Also retain a canonical-tag negative
control requiring the unsupported historical target recipe to refuse without
mutation. This keeps the historical-image resolver required for the three-release
retention proof. Do not skip the criterion, alter historical source bytes, or
claim that a historical renderer gained new CI behavior.

## Qualification fixture boundaries

The ordinary installer identity witness must remain observable by its existing
bootstrap criteria. Upgrade fixtures must use the exact historical workflow
projection they claim; witness instrumentation cannot silently redefine those
authenticated bytes. Keep these fixture purposes explicit at their shared owner.
Current and evaluator-modified archive images must also have distinct file paths;
assert each file still contains the exact bytes of the image it represents.

The report cancellation criterion must observe a completely published process-ID
record before parsing and signalling it. File creation alone is not a completed
write handshake. Preserve the existing cancellation deadlines and process-exit
assertions.

## Earlier copied-Go initializer recipe

The genuine predecessor regression exposed a second refusal after mode handling
was corrected: the immutable initializer at
`95486ecdd3868a319a0e6362ee99cafd8f154200` predates the copied-Go vector at
`6b8f08c820c347a9c9d12c4afd00b3f7e6aca547`. The earlier generation has ten
initializer production files; the later one has twelve, and five shared members
have different bytes. Its workflow copy/substitution behavior remains equivalent;
its original staging path also produces mode 0600.

Qualify this predecessor with its own complete frozen source vector. Pass that
vector through the existing exact hash, member-set and source-asset validation,
then reuse the copied-workflow projection and its 0600 mode. Never make missing
members optional, match only a subset, infer a generation from its release name,
or accept an unknown initializer. Bind the oracle to the immutable Git objects;
require changed, missing and unexpected member controls as well as successful
public upgrade and reverse restoration from the actual predecessor initializer.

Qualification receipts on 2026-11-12 UTC:

- Before the historical corrections, the complete `make go-runtime-source-check
  GO_RUNTIME_TEST_GROUP=ci-migration GO_RUNTIME_TEST_TIMEOUT=25m
  GO_RUNTIME_TEST_PROCS=auto` gate reported `15 Passed | 0 Failed` in 842.239
  seconds; its lint and security checks passed. Linux hosted migration also passed.
- The mode correction's race run of `go test -race -count=1 -v -timeout=5m
  ./internal/installation ./internal/releasecmd -ginkgo.fail-on-empty
  -ginkgo.no-color` reported 83 installation and 2 release-command criteria passed.
- The combined ten-criterion public regression reported `9 Passed | 1 Failed`
  in 785.398 seconds. The sole failure was the earlier initializer recipe above;
  it failed at read-only preview before the new apply/rollback assertions.
- The expanded immutable copied-workflow oracle, run with `go test -race
  -count=1 -v -timeout=3m ./internal/installation -ginkgo.focus='recognizes copied workflows
  from the frozen pre-generated Go initializer recipe' -ginkgo.fail-on-empty
  -ginkgo.no-color`, reported `0 Passed | 1 Failed` in 0.151 seconds before adding
  that vector: expected `retire`, observed `preserve`.
- With the complete earlier vector admitted through the strict matcher, the same
  full internal race command reported `83 Passed | 0 Failed` in 29.659 seconds
  and `2 Passed | 0 Failed` in 0.001 seconds.
- The final genuine historical path ran with `go test -race -count=1 -v
  -timeout=15m ./acceptance -ginkgo.focus='requires fresh authenticated installer
  rebootstrap when the immutable predecessor rejects newer compiler provenance'
  -ginkgo.fail-on-empty -ginkgo.no-color
  -ginkgo.json-report=../.factory/ci-mode-evidence/historical-rebootstrap-green.json`
  under command-scoped `caffeinate -i` and the explicit historical compiler.
  It reported `1 Passed | 0 Failed | 3458 Skipped` in 109.018 seconds, including
  forward application, inert backup, independent rollback and original mode/runtime
  restoration with distinct archive identity assertions.

These receipts do not claim final-head hosted qualification; that remains a
separate gate after the completed correction.

## Observe the complete unfinished publication interval

Hosted Linux qualification of `b79e48c` passed fourteen transaction criteria and
missed the reverse interruption witness in one. The forward witness was observed
in `applying`, but the reverse owner completed normally before the initial poll
observed its narrower `applying` interval. Initial observation ignored `checking`
and `metadata_prepared`, although the stopped-owner observation already accepted
those unfinished phases and reverse validation still held pending intent.

Use the same finite unfinished-phase predicate for both observations. Require
the requested direction, the exact applied/reversed workflow row, the restored
original bytes for reverse publication, and the pending marker before stopping.
Then observe actual SIGSTOP and reread that complete proof before SIGKILL and
reaping. A committed record, missing pending marker or exited owner never proves
interruption. This supersedes the initial-applying-only sampling restriction in
the earlier fixture receipt; it does not change product behavior, extend deadlines,
add test hooks, or permit successful completion to stand in for interruption.

Observed qualification on 2026-11-12 UTC: the Ginkgo CLI command used
`--procs=1 --race --timeout=15m --repeat=2 --fail-on-empty --no-color` with the
two existing interruption entries selected. It completed `2 Passed | 0 Failed`
in 303.130 seconds, then `2 Passed | 0 Failed` in 306.758 seconds. The third
forward case passed before the shared runner deadline interrupted reverse recovery;
the aggregate command exited 1 with `Suite Timeout Elapsed` after
`15m1.814212958s`. This was not a passing aggregate command.

Only the unfinished reverse entry was rerun under the original limits:
`go test -race -count=1 -v -timeout=15m ./acceptance
-ginkgo.focus='recovers actual interrupted workflow publication through emitted archive recovery arguments reverse restoration'
-ginkgo.fail-on-empty -ginkgo.no-color
-ginkgo.json-report=../.factory/ci-mode-evidence/interruption-reverse-third.json`
under command-scoped `caffeinate -i`. It reported
`1 Passed | 0 Failed | 3458 Skipped` in 243.159 seconds. Across these commands,
three forward and three reverse cases completed successfully. Reverse stopped
witnesses included `checking`, the reversed row, exact original workflow bytes
and pending intent; every completed case retained signal/reap and recovery checks.
Final `go vet ./...` exited 0 without output; configured
`golangci-lint run --config packs/go/.golangci.yml ./...` reported `0 issues.`.

## Bound handoff qualification by the exercised boundary

Hosted run `43259987561` at `851fe93` passed both CI-migration partitions,
but the macOS release-handoff partition exhausted its shared 25-minute suite
budget: `50 Passed | 2 Failed | 3407 Skipped`, with both failures reporting
`Suite Timeout Elapsed`. Its Go-recovery `BeforeAll` was inside a legacy Bash
initializer whose result that group no longer consumes. The legacy/init group
also built a distinct Go predecessor that only the Go-recovery group consumes.

Prepare legacy templates only for the legacy/init boundary and the distinct Go
predecessor only for Go recovery. Retain genuine authenticated native initialization
inside the Go recovery cases, immutable legacy sources, every assertion and the
existing child deadlines. Do not substitute a copied or hand-edited installation
for the actual initializer.

The same report attributes 684.739 seconds of worker time to twelve completed
CI-checkout criteria; seven further CI-checkout criteria had not run. Separate
the exact `CI checkout preparation` title family into a `release-ci` source gate
and matrix job. The original handoff partition retains its four ordered fixture
groups and their scheduling priorities. The independent CI boundary retains its
existing project fixtures and assertions. Preserve the current maximum of two
handoff workers, 25-minute suite limit, 30-minute job limit, race checks and quality
gates in both partitions. Require exhaustive disjoint selection of 44 handoff and
19 CI-checkout criteria, with the full acceptance census unchanged at 3459.
Exercise failure propagation, report names, worker selection and the real Ginkgo
discovery paths; do not infer coverage solely from the workflow matrix text.

These are decisions for the next qualification run, not measured timing savings
or a claim that final-head hosted qualification has passed.

## Isolate competing local Go upgrade qualification

Run `43305598632` at `6d64d6d` passed both new handoff partitions on Linux
and macOS, including all 44 handoff and 19 CI-checkout criteria. The separate
macOS local-upgrade job then reported one failed versioned Go predecessor case
and ten passes. Its failure occurred about eight minutes into the suite, on the
fresh 90-second watchdog around forward local apply, rather than the suite limit.

The two Go predecessor criteria intentionally rebuild selected and predecessor
sources through the public commands. Keep those actual builds, compiler-count
assertions, native initialization, rollback and identity checks. Cache links are
already shared through the evaluator's private HOME; bypassing compilation or
changing the packager's hermetic environment is not the correction.

Place the exact `Go explicit local source upgrade upgrades an exact Go predecessor
and restores its original native identity` family in a `local-upgrade-go` source
gate with one worker. Keep nine ordinary local-upgrade criteria at the existing
worker cap and the two recovery criteria in their existing separate gate. The
Go-only fixture must prepare the real source/image inputs without initializing
an unused legacy installation. Require exact identities, exhaustive disjoint
9/2/2 selection, unchanged 3459 total discovery, forced one-worker execution,
separate reports and failure propagation through explicit and all/default runs.

This removes competing test work from the failed command's runner; it is not a
measured claim about the time spent in an individual compiler or packaging stage.
Retain the 90-second command deadline, 25-minute suite deadline and 30-minute job
deadline. Add elapsed time, the command's actual run error, process exit state and
context error to watchdog diagnostics before asserting. A buffered completion
message alone cannot establish completion before the deadline: the default
CommandContext cancellation kills the direct caller, while an independently
supervised selected child may still own its output handles. Do not reinterpret
that message as successful exit or change cancellation semantics without an
owned-child negative control. Production batching of Git blob reads is a separate
performance boundary requiring its own strict size, framing and ownership proof.

## Quiesce Git maintenance in disposable checkout fixtures

Run `43320430957` at `2eb4feb` passed 38 checks, including both isolated Go
upgrade jobs. Its macOS release-CI job `131539372041` reported 18 passes and
one snapshot error: `.git/objects/maintenance.lock` disappeared between directory
enumeration and `lstat` while checking refusal preservation. This is a concurrent
Git maintenance artifact, not evidence that the command changed project assets.

Disable automatic maintenance and garbage collection in these disposable fixture
repositories before their first commit. Pass the same repository-local settings
through `git clone --config` before fetching and checkout, rather than configuring
the clone after it can already start maintenance. Keep the full filesystem
snapshots, including `.git`, and every existing refusal assertion unchanged.
Do not add snapshot retries, ignore disappearing files or alter user repositories.

The official [maintenance documentation](https://git-scm.com/docs/git-maintenance)
states that `maintenance.auto` controls automatic invocation and that maintenance
can detach. The official [clone documentation](https://git-scm.com/docs/git-clone)
states that clone configuration applies before fetch and checkout. The
[garbage collection documentation](https://git-scm.com/docs/git-gc) confirms that
`gc.auto=0` disables automatic collection heuristics. All three were fetched on
2026-11-13 UTC. Check the local fixture settings and rerun the actual
refusal criteria and full release-CI group; preserve the 3459-criterion census.

The added local-setting assertion failed before the fixture correction:
`go test -race -count=1 -v -timeout=10m ./acceptance
-ginkgo.focus='preserves customized, dirty, unsafe and already-owned checkout inputs on refusal'
-ginkgo.fail-on-empty -ginkgo.no-color
-ginkgo.json-report=../.factory/ci-mode-evidence/maintenance-red.json`
reported `0 Passed | 1 Failed | 3458 Skipped` in 54.942 seconds. Its missing
local `gc.auto` read returned 1. This proves the missing setup control; it is
not a local reproduction of the disappearing maintenance process.

After configuring the disposable repositories, the full
`make go-runtime-source-check GO_RUNTIME_TEST_GROUP=release-ci
GO_RUNTIME_TEST_TIMEOUT=25m GO_RUNTIME_TEST_PROCS=auto` behavioral run reported
`19 Passed | 0 Failed | 3440 Skipped` in 276.964 seconds, including the unchanged
full-checkout refusal witnesses. Its following lint stage reported two
`ifElseChain` style findings in the new source-observation tests. The complete
Make gate therefore exited nonzero; finish lint/security after the mechanical
test cleanup rather than treating the behavioral result as a passing whole gate.

After the switch-only assertion cleanup and its passing native-entrypoint rerun,
standalone `golangci-lint run --config packs/go/.golangci.yml ./...` reported
`0 issues.`, `gosec ./...` reported `Issues : 0`, and `govulncheck ./...` reported
`Your code is affected by 0 vulnerabilities.` All three commands exited 0. The
vulnerability scan retained ten module-only advisories, with none in imported
packages or called symbols. These local results do not replace final-head CI.
