# Go replacement release readiness

## Stable v0.2.0

The operator selected the merge of [PR #189](https://github.com/anoop2811/software-factory-template/pull/189)
for v0.2.0. The [stable release](https://github.com/anoop2811/software-factory-template/releases/tag/v0.2.0)
identifies the downloads. The [publication workflow](https://github.com/anoop2811/software-factory-template/actions/runs/44789814756)
passed all 20 scenarios on Linux and macOS, on amd64 and arm64. The [downloadable reports](https://github.com/anoop2811/software-factory-template/releases/download/v0.2.0/factory-qualification-v0.2.0-44789814756.tar.gz) preserve the run, target, signer subjects and scenario outcomes.
The public installer pins the tag. Existing projects choose when to apply the upgrade.
See the [v0.1.6 upgrade guide](../UPGRADING.md) for preview, confirmation and recovery.

### Signed adopter pilot, 2026-11-20 UTC

The operator-approved pilot used a private application fixture. These are local
observations, with per-command logs retained on the operator’s machine.
On macOS arm64, a fresh Git project and a disposable copy of the supported
v0.1.6 adopter both ran the public installer with `--ref v0.2.0`. The bootstrap,
installation image and runtime archive passed checksum and GitHub attestation
checks for the tag and merge commit. Both installed binaries matched the one
in the signed image (SHA-256
`a68e7df6e0f59523c95a69d1319fd33d6e10618c696743ce0085c36970310c68`).

| Exercise | Observed outcome |
|---|---|
| Fresh init and confirmed v0.1.6 upgrade | Both installations reported `14 shipped gates proved, 0 failed`. The adopter preview had zero blockers and conflicts. The upgrade preserved all 103 application files, configuration, HEAD and index. |
| Temporary application feature | The installed `./factory check` passed at baseline, caught the new failing test, passed all 13 frontend tests after implementation, and passed the original 12 after reversal. The adopter's product files were restored. |
| Failed rollback and recovery | With a required legacy tool unavailable, rollback returned 2 and left all 295 observed active paths unchanged. After restoring the tool, forward recovery and a fresh rollback succeeded. The transaction committed, 195 legacy files matched their original content and modes, and the restored Bash metrics command returned valid JSON. The root ignore-file update retained the private lock. |

The tagged source also passed 28 inventory and historical compatibility specs.
The fixed conversion inventory contains 21 command adapters, seven shell
libraries and four retired Python runtime modules. The original adopter checkout
was outside the exercise. Earlier pilot findings and fixture scope are recorded
in the [adopter pilot history](RC4_ADOPTER_PILOT.md).

The entries below record the checklist at earlier delivery milestones.
Their open items and installer pins describe the state at each milestone.

## Completed source prerequisites

Historical snapshot before stable promotion.

- Native source commands exist for init, doctor, report, metrics, review-lane,
  configuration migration, budgets and loops.
- Source-built Go `selftest` shares the installed native engine for fourteen
  required gate violation/healthy-control pairs, with separate source/executable
  custody and native cancellation. The broader historical shell regressions
  remain until the controller/proof cutover; this does not activate the source
  launcher or close a release gate. See
  [ADR 0120](../adr/0120-native-source-gate-proof.md).
- Explicit committed-local initialization packages clean source and runs its exact
  selected Go initializer; [ADR 0116](../adr/0116-explicit-go-local-initialization.md)
  records focused source qualification. This does not activate ordinary defaults.
- Ordinary Go initialization selects an authenticated packaged release, with
  explicit ref/environment precedence and admitted installed-version reuse. The
  prepared source default v0.2.0 refuses until published; public stable
  activation remains separate. See
  [ADR 0118](../adr/0118-default-packaged-go-initialization.md).
- Ordinary Go upgrade selects an authenticated stable release, preserves explicit
  selector ownership, refuses downgrades and gives the transaction its own
  admission without retaining an installed shared lease. The prepared v0.2.0
  default does not claim publication or activate the source bootstrap. See
  [ADR 0122](../adr/0122-default-packaged-go-upgrade.md).
- Explicit committed-local upgrade uses the selected build's own planner and
  whole-installation transaction, with exact Go predecessor reconstruction,
  preserved backups and replayable local recovery inputs. See
  [ADR 0117](../adr/0117-explicit-go-local-upgrade.md). The final integrated
  release rehearsal remains a separate gate.
- Declared obsolete-cache retirement uses the same transaction and ignored inert
  backups for the 83 unused files actually selected by a qualified older Go
  installation. Two consumed cache files remain; edited, unknown and unowned
  leftovers are preserved. Historical 141-row and minimal 58-row records retain
  their original source generation. See
  [ADR 0123](../adr/0123-owned-obsolete-initializer-cache-retirement.md). This
  source prerequisite does not activate bootstrap or close the final cleanup audit.
- Ordinary shell-installer init/upgrade requests with an exact stable Go ref
  reuse authenticated bootstrap acquisition and the native stable owner. The
  shell checks stable publisher metadata before first execution and carries one
  explicit ref into Go. Bare fetch stays Git-only; the public v0.1.6 pin remains.
  See [ADR 0124](../adr/0124-ordinary-authenticated-go-installer-bootstrap.md).
- The tracked developer launcher builds and enters the existing native Cobra
  command; direct init/upgrade scripts forward to the same owners. Branch
  initialization retains the selected committed source, and the copy hook uses
  the initializer's actual shared Go inventory. Existing installed launchers
  remain compiler-free. See [ADR 0125](../adr/0125-native-initializer-copy-inventory-gate.md)
  and [ADR 0126](../adr/0126-go-source-launcher-and-direct-init-upgrade.md).
  Direct report/metrics conversion is the bounded R3 follow-up in
  [ADR 0127](../adr/0127-go-source-observability-entrypoints.md).
  Direct OpenCode/Claude/Codex synchronization continues R3 in
  [ADR 0128](../adr/0128-go-source-harness-sync-entrypoints.md).
  Direct doctor conversion reuses the native diagnostic owner in
  [ADR 0129](../adr/0129-go-source-doctor-entrypoint.md).
  Direct prerequisite discovery reuses the native tool checks and removes its
  obsolete Python requirement under
  [ADR 0130](../adr/0130-go-source-prerequisite-entrypoint.md).
  Direct golden evaluation reuses the existing native evaluator under
  [ADR 0132](../adr/0132-go-source-golden-evaluation.md).
  Remaining direct commands/helpers and historical proof cleanup are enumerated
  in [the R3 cutover checklist](R3_CUTOVER.md). Source changes do not by themselves
  close a release gate; review and hosted qualification are recorded separately.
- Artifact verification, deterministic selection, local packaging and authenticated
  inert staging exist for the original runtime-only bundle.
- Confined adoption/preview, ignored recovery creation and read-only restoration
  planning exist for the bounded six-asset G2 scope.
- Transition exclusion, live restoration, durable publication and interrupted
  recovery source engines are qualified and merged.

The whole-installation consumer and explicit stable Go installer refs can select
an authenticated Go runtime. Current source and direct init/upgrade entrypoints
use Go; the public installer default remains v0.1.6. Bare fetch executes no
downloaded code. Stable activation and final release qualification remain separate.
Explicit candidate delivery is described in [GO_CANDIDATE_HANDOFF.md](GO_CANDIDATE_HANDOFF.md).

## Fourth release candidate (historical)

[v0.2.0-rc.4](https://github.com/anoop2811/software-factory-template/releases/tag/v0.2.0-rc.4)
was published on 2026-11-16 UTC from
[PR #185's reviewed merge](https://github.com/anoop2811/software-factory-template/pull/185)
`c51e0ae520839076c7245f2de7e396e4c3e546fe`. The PR passed all 46 checks;
the merged source passed all 47 reported checks, including four native bundles.
The release remains a prerelease and v0.1.6 remains Latest stable.

R5 is complete: live published-asset qualification passed five criteria on each
of four native targets, twenty criterion executions in total, in
[release run 43958890935](https://github.com/anoop2811/software-factory-template/actions/runs/43958890935).
Each report confirms the approved minimum OS and authenticates its three release
subjects against the selected source. The
[RC4 evidence record](PUBLISHED_RELEASE_QUALIFICATION.md#fourth-published-candidate-result)
records those actual outcomes separately from source CI. **Two release gates
remain: R6 adopter pilot/signoff and R7 stable activation/publication.**

The [published RC4 adopter pilot](RC4_ADOPTER_PILOT.md) is in progress.
Forward upgrade, installed application checks and interrupted forward recovery
passed, but failed rollback preparation stranded its transaction journal and a
customized ignore file exposed an addable private transition lock. The rollback
correction merged in [PR #187](https://github.com/anoop2811/software-factory-template/pull/187).
[PR #188](https://github.com/anoop2811/software-factory-template/pull/188)
merged the lock correction. The [merged-source pilot](RC4_ADOPTER_PILOT.md#rerun-with-both-fixes-in-merged-source)
passed forward upgrade, lock visibility and the temporary feature exercise.
Rollback refused with legacy Python unavailable. After restoring Python, public
recovery and a fresh rollback both succeeded. R6 still
requires evidence from signed release assets containing both fixes and the
operator's explicit acceptance of the pilot results and agreement on the Q5
evidence period and exit criteria.

### Earlier RC3 qualification (historical)

[v0.2.0-rc.3](https://github.com/anoop2811/software-factory-template/releases/tag/v0.2.0-rc.3)
was published on 2026-10-26 UTC from reviewed merge
`95486ecdd3868a319a0e6362ee99cafd8f154200`. Its tree matches PR #138's reviewed
head, which passed all eighteen checks. Release run 40275363119 passed four
builds, authentication of twelve subjects, fourteen uploaded assets and all
**twenty actual live scenario criteria** across the four native targets.
See [the exact RC3 results](PUBLISHED_RELEASE_QUALIFICATION.md#third-published-candidate-result).

The observed systems are Ubuntu 24.04.5 on both architectures, macOS 14.8.9
arm64 and macOS 15.7.9 amd64, under the then-approved macOS 14 floor. Decision
128 (2026-10-30) now sets macOS 15+ on both architectures; macOS 14 is no longer
supported. At that checkpoint, a new candidate needed native qualification at
the revised minimums. RC4 supplies that evidence above; RC3's original evidence
and immutable assets remain historical. v0.1.6 remains latest stable.

RC1 and RC2 remain unchanged and marked unqualified for migration. RC3 corrects
their recorded failures; its passing published results are separate evidence.
The earlier merged-source macOS base job independently repeated an isolated
Git-init five-second timeout in its healthy archive-subject control. That failing
revision's fixture selected a different Git from the qualified runner/live lane.
PR #139 subsequently merged after all eighteen exact-head checks passed. It
selects and records the evaluator Git and requires genuine pre-reap fixture
witnesses with independent bounded cleanup. These source-fixture corrections do
not establish the original host delay's cause or change RC3's immutable runtime.
Decisions 114-116 retain both the original failures and later source evidence.

A subsequent Linux source run exposed inherited-terminal startup ordering. The
child could inspect/read its terminal before the parent's post-Start foreground
handoff. Decision 117 records a deterministic real-terminal regression and a
child-side pre-exec correction, plus the observer's actual-foreground prerequisite
before Ctrl-Z. That runtime correction requires a newly reviewed candidate and
fresh published qualification. RC3's original twenty passes do not qualify a
later binary; its source, tag and assets remain unchanged.

## Required release gates

Historical checklist from candidate qualification. See the stable result above.

- [x] Complete committed source-image bundle and fixed legacy reference catalogs,
      with shared validation and authenticated closed-world payload custody.
      ADR 0097 source implementation: 98 new criteria and 72 unchanged V1 criteria
      passed locally. PR #124 merged after all thirteen checks passed, including
      complete Linux/macOS source gates and four native V1/image target jobs.
- [x] Complete reviewed installation selection and retain/replace/retire map for
      each supported predecessor and the Go target. Source/test/development blobs
      in the image are not all installation assets and grant no ownership.
- [x] One whole-installation upgrade transaction retains exclusion and pending
      admission across all changed files, durable ignored backup, candidate checks,
      activation and the durable installed-version result.
- [x] Public upgrade, explicit rollback and interrupted recovery reuse that
      transaction and independently qualified images; later edits and unsupported
      predecessor state remain preserved conflicts.
- [x] Complete remaining factory-owned orchestration and adapter/dependency audit.
      Preserve required public/sourceable paths as thin adapters. Intentional
      validation gates, product build files and native host formats remain.
      [PR #126](https://github.com/anoop2811/software-factory-template/pull/126/checks)
      merged after all sixteen checks passed, including a
      [completed adversarial review of its final source head](https://github.com/anoop2811/software-factory-template/pull/126#issuecomment-6244742505).
- [ ] Init/install/bootstrap and normal upgrade select a verified packaged Go
      runtime without requiring Go or Python for shipped factory behavior.
- [ ] Retire only unchanged, explicitly owned/adopted obsolete implementations
      after compatible Go activation. Backups are inert and ignored; unknown,
      edited, held or unsafe entries are individually reported and preserved.
- [x] Later distinct successful forward releases prune eligible older recovery
      sets while keeping the immediately preceding usable installation. Same-release
      retries, rollback and local builds never advance retention.
      Source implementation and qualification are in [PR #132](https://github.com/anoop2811/software-factory-template/pull/132)
      and [BACKUP_RETENTION.md](BACKUP_RETENTION.md). This source gate does not claim
      human merge, default selection or release publication.
- [x] Packaged acceptance passes natively on Linux amd64/arm64 and macOS amd64/arm64
      at the approved minimum operating systems, including actual release authenticity.
      RC4 run 43958890935 passed five live criteria per target, twenty in total,
      with all four `minimum_os_qualified` records true.
- [ ] Complete the mandatory manual adopter pilot and record its project,
      execution evidence and agreed exit criteria before default cutover.
- [ ] Publish the Go replacement release only after the above gates pass and the
      final dependency/reference audit finds no unclassified active remnants.

## Installed copied-hook setup obligation

R3 group 4 inspection found that the existing generated `scripts/pre-push-check.sh`
assumes its installed `scripts/` location. Copying it to `.git/hooks/pre-push`, as
the initializer previously recommended, resolves `.git/factory` and fails. Before
stable activation, R4 must provide and qualify a valid installed full-check hook
setup and correct that advice while preserving frozen predecessor recipes.
The source-only copied-hook correction in [ADR 0138](../adr/0138-go-source-check-orchestration.md)
does not qualify installed copies. The tracked `.githooks/pre-push` remains the
separate direct-main-only hook, not a replacement for a full-check hook.

Decision 157 / [ADR 0142](../adr/0142-installed-full-check-hook-setup.md) records
the bounded correction: an explicit user-owned hook calls `./factory check`
from Git's worktree root. The [setup guide](../HOOKS.md#full-pre-push-checks)
preserves existing hooks and configuration; init links to it and doctor scopes
its report to tracked-path selection. Qualification and merge status are recorded
in that ADR. This obligation is separate from the integrated rehearsal below.

## Fresh-checkout CI delivery obligation

PR #173 merged the installed full-check hook setup correction. A fresh clone
still omits ignored runtime custody, and its public adapters bind the original
platform's executable digest. Decision 158 / [ADR 0143](../adr/0143-ci-checkout-preparation.md)
adds an explicit native preparation boundary for a disposable job checkout.
See [CI preparation](CI_PREPARATION.md) for its scope and requirements.

PR #174 merged native preparation and its watchdog correction. Decision 160 /
[ADR 0144](../adr/0144-ci-installer-bootstrap.md) adds the next bounded installer
entrypoint: a separately trusted installer authenticates the selected host owner
before invoking native preparation. Qualification of that route is distinct from
generated workflow delivery and preservation-aware existing-workflow integration.

PR #175 merged that installer entrypoint. Decision 161 /
[ADR 0145](../adr/0145-installed-ci-workflow-rendering.md) adds explicit native
workflow rendering for review, with pinned bootstrap and installed checks. It
does not publish workflow files or repair older generated jobs. PR #176 merged
that renderer. Decision 162 / [ADR 0146](../adr/0146-initialized-installed-ci.md)
implements automatic ordinary new-project CI with selected-pack setup and
preserved application workflows. It also preserves semantic OpenCode permission
ordering so initialization does not need an extra repair synchronization. The
four independent init-to-CI criteria and three ordering/scheduler criteria passed
locally, and PR #177 merged after all 34 final-head hosted checks passed.
PR #178 merged Decision 163 / [ADR 0147](../adr/0147-installed-ci-migration-preview.md),
which supplies read-only ownership planning for existing workflows. Decision 164 /
[ADR 0148](../adr/0148-transactional-installed-ci-migration.md) adds the next bounded
source change: authenticated upgrade includes those workflow changes in the same
consent, ignored backup, publication, recovery, rollback and retention transaction.
Customized selected workflows remain preserved blockers. PR #180 merged on
2026-11-13 as `2a860fc0f6109f8fdb5e5c5f7a896008994931f3`, after all 46 checks
passed on `a03a024316419d450aaa6f6e7fca8746406efa5e`. Final-head adversarial
review covered all 88 files and 274 fragments in 2m58s, with all 67 notes answered.
These are the observed prior PR results, not qualification of later changes.

Decision 169 / [ADR 0153](../adr/0153-authenticated-review-client-bootstrap.md)
implemented authenticated standalone review-client acquisition. PR #181 merged
on 2026-11-14 as `3794e6cc8fa66eb02504e753b0ad11955f5cf85a`; its final-head Go
and adversarial runs passed, with the review job taking 2m51s. Decision 170 /
[ADR 0154](../adr/0154-installed-review-workflow-migration.md) governs the dependent
integration: pin that installer in generated installed review workflows and
update exact predecessor-owned enabled workflows transactionally. PR #182 merged
on 2026-11-15 as `8772e919a66d625085a1f69320f7dd0afc2e59eb`. Bootstrap
alone does not qualify those installed workflows. Decision 172 /
[ADR 0156](../adr/0156-integrated-adopter-rehearsal.md) connects the remaining
fresh/migrated rehearsal. PR #183 merged on 2026-11-15 as
`509d4bbc01743fa3900247273d082171e1bf3a18`, with the same tree as its reviewed
head and 46 passing current checks. The
[integrated rehearsal report](INTEGRATED_ADOPTER_REHEARSAL.md#final-hosted-evidence-and-merge)
records both platforms, review dispositions and the separate initial watchdog
failure/passing repeat without claiming its cause is known. R4 is complete.
R5 published qualification is now complete; R6 adopter pilot and R7 stable
activation remain the two release gates. Decision 173's live-evaluator
child-ownership and installer startup-cancellation corrections merged in PR #184 on 2026-11-16 as
`959ebbc2a661fe7911f2770b1c5746b54c287249`, with the same tree as the reviewed
head and 46 passing checks. Decision 174 advanced the installed review workflow's
immutable installer pin to that correction. PR #185 completed its consumer
qualification and review, and merged before RC4 source selection. These were R5
prerequisites, not additional release gates. A PR count is not release progress.

Runtime preparation, installer/bootstrap integration, adopter-specific workflow
generation and preservation-aware existing-workflow integration are distinct
obligations. The factory's source-development CI includes checks that are not
shipped to adopters; copying it is not a complete installed CI solution. The
integrated fresh-project and migrated-adopter rehearsal exercised those actual
job paths from clean checkouts without Go or Python for the factory. Published
qualification and the adopter pilot remain separate obligations.

## Observable release rehearsal

An adopter starting from v0.1.6 can use the normal upgrade flow. The selected Go
binary and required template assets match a verified source revision. Existing
configuration, application files, custom hooks, native permissions and consumed
runtime history remain intact. Every retained public entrypoint reaches its one
declared implementation.

Interruptions at verification, backup, mutation, validation, activation and
cleanup boundaries either retain a complete compatible installation or stop
ordinary work with a concrete recovery action. Rollback does not execute saved
scripts, rewind accounting or overwrite later edits. Successful later-release
retention has visible file/byte totals and preserves all declared exceptions.

No model is called as an upgrade or health test. Existing paid/optional choices
and budgets remain explicit.

## Installed doctor correction

Historical snapshot of the work remaining after the doctor correction.

An isolated local adopter rehearsal exposed a sourceable-library permission
mismatch in native doctor. Decision 112 corrects it while retaining executable
requirements for invoked hooks. Independent criteria include an actual qualified
installation at its declared 0644 library mode; local permission and doctor suites
pass. See [the anonymized factory evidence](ADOPTER_REHEARSAL.md).

PR #137 merged after all eighteen exact-head checks passed. Its merged tree is
identical to the reviewed head used for RC2. The new published outcomes above
remain distinct from that source qualification. RC4 now supplies the minimum-OS
evidence; the mandatory operator pilot, default cutover and final audit remain
open requirements within R6 and R7.

## Operator input at the rehearsal milestone

An isolated rehearsal has run against the operator-selected adopter; its detailed
records remain local. A genuine v0.1.6 fixture migrated, while the actual older
customized installation remained preserved ownership blockers. Q1 requires
preservation of unsupported installations, not forced migration of that older
checkout. Its reconciliation is separate adopter work. Decision 111 permits a
separately labeled supported installation containing unchanged real application
code as the pilot fixture.

Q5 still requires an agreed evidence period/exit criteria and operator acceptance
of a representative supported-project pilot using the current candidate. Earlier
local rehearsal findings are historical evidence; passing application tests alone
does not establish a completed current-release upgrade/workflow/recovery pilot or
permit default cutover. No minimum calendar duration is specified.

## Current installation-consumer qualification

PR #125 merged on 2026-10-17 after all fifteen reported checks passed. It adds
the opt-in go-hybrid-v1 whole upgrade, rollback and interrupted-recovery paths
specified in docs/adr/0098-whole-installation-upgrade-and-rollback.md:14. This
closes those three source prerequisites above; default bootstrap and replacement
release gates remain open. For PR #125, the advisory model review skipped the oversized diff
and Copilot had no quota, so neither is claimed as completed model review.

The completed orchestration implementation and its qualification are tracked in
[ORCHESTRATION.md](ORCHESTRATION.md). Do not count a
new interface, inventory or review correction as another source-plan package.
The fixed denominator remains thirty; no new percentage is inferred here.

## Published-release qualification lane

[Published-release qualification](PUBLISHED_RELEASE_QUALIFICATION.md) defines the
explicit attested candidate lane under [ADR 0107](../adr/0107-published-release-qualification.md).
Its implementation and ordinary hermetic contract checks do not close the native
published-release gate. Record actual published outcomes and exact platform/OS
evidence separately, including both architectures at the current macOS 15 minimum.
The mandatory adopter pilot and default cutover remain independent requirements.
