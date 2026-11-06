# R3 source entrypoint and legacy runtime cutover

This is the finite remaining source-cutover inventory for the approved
[Go conversion specification](../../specs/001-go-runtime-conversion.md), not a
new feature roadmap. It was inspected on 2026-11-02 against merged PR #161
(`f3a0b65`), before the next R3 implementation. The Go owners below already
exist; their presence does not qualify the remaining direct source paths.
Progress rows are updated separately from that fixed baseline inventory and name
their later qualification decision. They do not attribute new work to PR #161.

The inventory contains **14 executable entrypoints, 7 sourceable libraries and
4 Python runtime modules**. These 25 paths are a cleanup checklist, not equal
units of implementation effort or a release-completion percentage. Historical
test custody is tracked separately in
[R3 proof retirement](R3_PROOF_RETIREMENT.md). R3 completion does not publish
0.2.0 or close the independent [release gates](GO_RELEASE_READINESS.md).

## Already completed direct R3 commands

These seven paths are outside the remaining counts above. Their source cutovers
are recorded in the merged decisions; this document does not claim a new run of
their checks.

| Paths | Go owner | Merged source evidence |
|---|---|---|
| `scripts/factory-report.sh`, `scripts/factory-metrics.sh` | `reportcmd`, `metricscmd` | PR #158; [ADR 0127](../adr/0127-go-source-observability-entrypoints.md) |
| `scripts/sync-opencode.sh`, `scripts/sync-claude.sh`, `scripts/sync-codex.sh` | `synccmd` | PR #159; [ADR 0128](../adr/0128-go-source-harness-sync-entrypoints.md) |
| `scripts/factory-doctor.sh` | `doctorcmd` | PR #160; [ADR 0129](../adr/0129-go-source-doctor-entrypoint.md) |
| `scripts/prereq-check.sh` | `prereqcmd` | PR #161; [ADR 0130](../adr/0130-go-source-prerequisite-entrypoint.md) |

The source launcher and direct init/upgrade adapters belong to completed R2
source work and are not counted again here. Installed Go layouts already
generate native command and library adapters under
[ADR 0099](../adr/0099-native-factory-orchestration.md); R3 must establish the
corresponding source routing and remove its duplicate implementations.

## Remaining executable entrypoints

Keep each public path as a thin adapter to its existing owner. Preserve caller
cwd, literal arguments, environment, streams and status under the relevant
contract; no missing-owner case may fall back to a saved legacy implementation.
The checklist records source qualification and review, not merely written files.

| Done | Path | Existing Go command / package | Function boundary and remaining evidence |
|---|---|---|---|
| [x] | `scripts/golden-task-eval.sh` | `eval golden` / `evalcmd` | Merged [PR #163](https://github.com/anoop2811/software-factory-template/pull/163) on 2026-11-03, Decision 147 / [ADR 0132](../adr/0132-go-source-golden-evaluation.md): independent RED/GREEN, direct/Make routing and no-Python scoring; all 32 hosted checks passed on [`a34b046`](https://github.com/anoop2811/software-factory-template/commit/a34b0462881efb9f8f4f9e0229fcfab079db5b0b). Merge [`b33ab2f`](https://github.com/anoop2811/software-factory-template/commit/b33ab2fa5a602355a9348fd497b7466c2514aaa6) has the same tree. |
| [x] | `eval/runners/mock.sh` | `eval run mock` / `evalcmd` | PR #164 / ADR 0133: deterministic pass/fail/cheat behavior and actual golden integration; qualification below. |
| [x] | `eval/runners/claude.sh` | `eval run claude` / `evalcmd` | PR #164 / ADR 0133: literal operands, model/log behavior and owned nested shutdown through fake CLI qualification. |
| [x] | `eval/runners/codex.sh` | `eval run codex` / `evalcmd` | PR #164 / ADR 0133: the same boundary under Codex's invocation contract. |
| [x] | `eval/runners/opencode.sh` | `eval run opencode` / `evalcmd` | PR #164 / ADR 0133: the same boundary under OpenCode's invocation contract. |
| [x] | `eval/runners/example-harness.sh` | `eval run example-harness` / `evalcmd` | PR #164 / ADR 0133: deliberately unwired example refuses without selecting a model. |
| [x] | `scripts/harness-structural-eval.sh` | `eval structural` / `evalcmd` | Merged [PR #165](https://github.com/anoop2811/software-factory-template/pull/165), Decision 149 / [ADR 0134](../adr/0134-go-source-structural-evaluation.md): source routing, real three-harness role controls, Make and four pack consumers; qualification below. |
| [x] | `scripts/adversarial-review.sh` | `review` / `adversarialreviewcmd` | Merged PR #166, Decision 150 / [ADR 0135](../adr/0135-go-source-review-entrypoints.md): source/HTTP routing and shipped workflow with fresh module-cache preparation; full source 71/3275 passed. All 32 final hosted checks passed. |
| [x] | `scripts/factory-review-lane.sh` | `review-lane` / `reviewlanecmd` | Merged PR #166, Decision 150 / ADR 0135: opt-in lifecycle, workflow ownership, caller roots and historical fixtures. Shared focused review/payload/workflow run passed 23 criteria; all 32 final hosted checks passed. |
| [x] | `scripts/factory-migrate-config.sh` | `migrate-config` / `migrateconfigcmd` | Decision 151 / [ADR 0136](../adr/0136-go-source-configuration-entrypoints.md): native source routing, dry-run/apply and historical fixture custody merged in PR #167; qualification below. |
| [ ] | `scripts/pre-push-check.sh` | `check` / `checkcmd` | Check orchestration: eight-stage ordering, real stdin, caller role, Git ranges and local-hook operand compatibility. |
| [ ] | `scripts/factory-budget.sh` | `budget` / `configuredcmd` | Controller boundary: preserve configuration, models, state and process ownership while removing Python dispatch. |
| [ ] | `scripts/factory-loop.sh` | `loop` / `configuredcmd` | Controller boundary: preserve checkpoints, budgets, opt-in execution and cancellation while removing Python dispatch. |
| [ ] | `scripts/selftest/run.sh` | `selftest` / `installedproof` | Proof-runner boundary: native public routing plus independent custody/coverage for the broader historical regressions; see the proof-retirement ledger. |

The five evaluation-runner rows merged in
[PR #164](https://github.com/anoop2811/software-factory-template/pull/164) on
2026-11-04. They have local implementation and independent
qualification under Decision 148 / [ADR 0133](../adr/0133-go-source-evaluation-runners.md).
Twenty-eight new criteria cover their source routing, native execution, nested
shutdown, failure reporting, source-bootstrap refusal, bounded receipt capture
and acknowledged adapter handoff. After hosted feedback, the actual local source
suite passed all 56 source criteria under race with two workers; the separate
native evaluation and partition run passed 49. Independent discovery found 3260
total criteria. Decision 148 records the earlier
qualification and the subsequent portability/ownership corrections separately.
All 32 hosted checks passed on
[`9c60eff`](https://github.com/anoop2811/software-factory-template/commit/9c60eff04d30ac0e45dcadcae77c70a1c1ae5b9a),
and review findings received responses before merge. Merge
[`9b7ef1f`](https://github.com/anoop2811/software-factory-template/commit/9b7ef1ff4f2990309e94af61a19839e94e56cebf)
has the same tree. One macOS source attempt failed an existing eight-second
fixture wait; its unchanged targeted rerun passed. The original stall was not
reproduced and its cause is unproven; this does not claim a timing correction.
These are not five new runtime capabilities: their native owners already existed.

The structural evaluator merged in PR #165 on 2026-11-04 as
[`aec8d12`](https://github.com/anoop2811/software-factory-template/commit/aec8d12b9c9495d8461ad6383221183d40d3d6c2).
Its final head `4d5fb70` passed all 32 hosted checks. Linux and macOS source jobs
each passed 63 criteria. The final advisory review covered all nine changed
files and twelve fragments, with findings answered. Copilot could not review
because of account quota. An unchanged macOS historical-job rerun passed after
three earlier historical failures; their cause remains unproven and no timing
fix is claimed. These are recorded prior qualification results, not a new run
made by this documentation update.

The review group merged in [PR #166](https://github.com/anoop2811/software-factory-template/pull/166)
on 2026-11-05 UTC as `5fabe9c`. Its final head `74cb382` passed all 32 hosted
checks; the unchanged macOS local-upgrade rerun passed after watchdog/deadline
failures on the first attempt. Their cause remains unproven. The final model
review completed all 21 files and 48 fragments in 126 seconds and received
per-finding responses. These are recorded results, not a new qualification run.

Across the fixed baseline and completed rows, 17 of 21 direct source
entrypoints and three of seven sourceable libraries are merged. Including the
four Python modules, that is 20 of 32 inventory paths (62.5%), with 12 remaining.
These are path counts, not effort-weighted implementation or release percentages.

Group 2 merged in PR #167 under Decision 151 / ADR 0136 as
`9f40f28219d2ea73cf69d86da470d7e763447953` on 2026-11-06 UTC. The final head
`4df37aa` passed all 32 hosted checks, including 79/79 macOS source criteria.
Both model-review rounds received per-finding responses; Copilot could not
review because of account quota. These are recorded results, not a new run.
Independent configuration/structural checks passed 134 criteria;
the final eight new source criteria passed after review corrections. Separate
native configuration/helper/scheduling, source-proof/recovery and transition
runs passed 355, 71 and 88 criteria respectively. Historical development checks
retain their assertions and passed 218 general, 3 optional-pack, 6 Java, 32 budget
and 40 loop cases. Enclosing source qualification and hosted review are recorded
separately in Decision 151. The four rows are now included in the merged count.
Group 3 begins under Decision 152 / [ADR 0137](../adr/0137-go-source-shared-helpers.md); its four
helper rows remain unchecked until enclosing qualification and merge. Seven new
source criteria passed after independent RED and review corrections; separate
helper/structural/source-init and native-helper/transition runs passed 15 and 38.
The measured census is 3290 total / 86 source criteria. Historical fixture suites
retained their assertions. Decision 152 distinguishes those completed checks
from earlier enclosing source-suite failures and hosted results. The final
default four-worker source gate passed all 86 criteria and its quality stages
after the documented cancellation-fixture startup correction. The subsequent
metadata/loader review correction passed 15 focused helper/configuration criteria
and the complete 86-criterion source gate in 129.248 seconds. Hosted qualification
of the correction commit and merge remain pending. Merging this group would
bring the inventory to 24/32 (75%), leaving eight paths in four R3 groups.
Installer cache-refresh timing retains its local bootstrap formatter: durations
over an hour remain minutes/seconds, for example `64m 0s`. Native factory timing
continues to use the hour form. This keeps elapsed reporting independent of
downloaded source execution.

## Grouped delivery order

Decision 150 groups the remaining paths by their consumers and compatibility
boundaries. Each PR includes affected fixture custody, checks and review; the
next group starts after the previous PR merges. Seven PRs is a delivery target,
not a guarantee that unresolved evidence cannot require a further split.

| Order | Group | Inventory paths |
|---|---|---:|
| 1 | Review execution and review-lane administration | 2 |
| 2 | Config migration, config/role readers and budget-config reader | 4 |
| 3 | Events, timing, color and hooksPath helpers | 4 |
| 4 | Check/pre-push orchestration | 1 |
| 5 | Budget command source cutover | 1 |
| 6 | Loop source cutover and four shared Python runtime retirements | 5 |
| 7 | Public selftest and final historical-test separation | 1 |

The Python loop imports the budget engine, so removing shared modules waits
until both public controllers and every historical consumer have a qualified
disposition. Each controller group closes its own proof-retirement rows rather
than deferring all historical coverage to the final selftest group. R4-R7 remain
separate release work after R3.

## Remaining sourceable libraries

Reuse the existing private Go bridge protocols. Caller-shell assignment and
operand expansion remain in shell where required; parsing and computation have
one Go owner. Sourcing an inert reader must not launch work. Source-time palette
and budget exports are intentional effects and require independent checks.

| Done | Path | Existing owner / bridge | Remaining compatibility evidence |
|---|---|---|---|
| [x] | `scripts/lib/config.sh` | `config`; `runtime/shell/readers.sh`, `runtime/shell/config.sh` | Decision 151 / ADR 0136: canonical native reads/plans through physical source transport; inert loading, caller attributes and real policy consumers qualified and merged in PR #167; all 32 final hosted checks passed. |
| [x] | `scripts/lib/roles.sh` | `roles`; `runtime/shell/readers.sh` | Decision 151 / ADR 0136: shared native role/tier reader, inert Bash/POSIX selection and explicit POSIX-owner refusal qualified and merged in PR #167; all 32 final hosted checks passed. |
| [ ] | `scripts/lib/events.sh` | `eventlog`, `helpers`; `runtime/shell/helpers.sh` | Decision 152 / [ADR 0137](../adr/0137-go-source-shared-helpers.md): native append/retention and real gate consumers; quiet missing-owner/bridge behavior. Implemented and independently qualified; hosted checks and merge pending. |
| [ ] | `scripts/lib/timing.sh` | `helpers`; `runtime/shell/helpers.sh` | Decision 152 / ADR 0137: native duration formatting, zero/unknown fallback and inert Bash/POSIX loading. Implemented and independently qualified; hosted checks and merge pending. |
| [ ] | `scripts/lib/color.sh` | `presentation`, `helpers`; `runtime/shell/helpers.sh` | Decision 152 / ADR 0137: canonical native palette/box, actual TTY and NO_COLOR, literal overrides and source-order ownership. Implemented and independently qualified; hosted checks and merge pending. |
| [ ] | `scripts/lib/hookspath.sh` | `githooks`, `helpers`; `runtime/shell/helpers.sh` | Decision 152 / ADR 0137: native actual-Git classification and literal paths; all four hook states observed through the source adapter. Implemented and independently qualified; hosted checks and merge pending. |
| [x] | `scripts/lib/budget-config.sh` | `config`, `roles`, `helpers`; `runtime/shell/helpers.sh` | Decision 151 / ADR 0136: shared native complete settings plan, model routes and source-time exports qualified and merged in PR #167; all 32 final hosted checks passed. |

## Legacy Python runtime retirement

Retire these active implementations only with their direct controller consumers
and fixture dependencies accounted for. Historical baseline bytes may remain in
isolated tests; they must not become installed or source-command fallbacks.
Removal from this repository grants no new deletion authority over adopter files.
Existing installation ownership, backup and customized-file rules still apply.

| Done | Path | Replacement and dependency to close |
|---|---|---|
| [ ] | `scripts/lib/budget.py` | Native budget engine; remove direct shell dispatch and explicitly freeze remaining historical budget fixtures. |
| [ ] | `scripts/lib/budget_adapters.py` | Native invocation/usage machinery; retain independent immutable parser and process oracles where still required. |
| [ ] | `scripts/lib/loop.py` | Native loop engine; remove direct shell dispatch and explicitly freeze remaining historical loop fixtures. |
| [ ] | `scripts/lib/runtime_transition.py` | Native transition guard; preserve mixed-runtime legacy evidence without keeping an active current Python controller. |

## Consumer and proof obligations

- Golden/structural entrypoints are used by Make and Go, TypeScript, Java/Gradle
  and Java/Maven pack workflows. Source CI must provision the source toolchain
  before its first newly native invocation; installed adopters use their binary.
- The historical shell controller copies current golden, review-lane and
  migration scripts. Its frozen dispatcher must receive coherent historical
  command/library bytes when these current paths become adapters.
- Budget/loop Python fixture builders copy current wrappers and `scripts/lib`.
  Existing historical-entrypoint materializers do not by themselves freeze all
  those bodies. Convert or explicitly freeze these consumers before deletion.
- Review request fixtures copy the current provider script and mock its legacy
  transport. Preserve every required behavior through native coverage or clearly
  identified historical custody; changing the wrapper alone is insufficient.
- Generated source review jobs must provision the source toolchain and locked
  modules before invoking native adapters. R4 must separately rehearse fresh
  adopter CI runners: Git checkout does not transfer an ignored local installed
  runtime. Local installed-command success alone does not qualify runner delivery.
- Public native selftest currently proves fourteen core gate pairs. That is not
  parity with all historical configuration, packs, upgrade, eval and reporting
  regressions. The [proof-retirement ledger](R3_PROOF_RETIREMENT.md) records the
  coverage decisions separately from source routing.
- Historical source images and installation recipes retain their immutable
  reconstruction contracts. Update current copy/reference inventories without
  reinterpreting frozen recipe generations or deleting their required evidence.

These are inspected dependencies and planned qualification obligations, not
claims that new tests have run. Update a row only with its actual implementation,
independent RED/GREEN evidence, review disposition and hosted result.

## R3 exit conditions

- [ ] Every path above has one classified disposition: qualified native routing,
      a justified thin sourceable adapter, or evidence-backed runtime retirement.
- [ ] No current public factory command or helper invokes an active Python
      factory implementation; unsupported/missing native owners fail explicitly.
- [ ] Each historical regression is independently replaced or retained as
      explicitly isolated test-only evidence with a rationale. No coverage is
      silently dropped and no historical fixture supplies a runtime fallback.
- [ ] Current CI, copy inventories, generated references and documentation agree
      with the retained paths and their owners; no active caller selects a removed
      implementation.
- [ ] Relevant source and installed checks, independent reviews and hosted checks
      pass for the completed boundaries. Final release rehearsal, native candidate
      qualification, adopter pilot and public default activation remain separate.

Intentional retained boundaries include validation gates, product build commands,
native harness formats, shell assignment/operand adapters and test-only historical
oracles. A Bash file extension or a Python oracle in an isolated test is not, by
itself, an incomplete runtime conversion.
