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
| [x] | `scripts/golden-task-eval.sh` | `eval golden` / `evalcmd` | Merged PR #163 on 2026-11-03, Decision 147 / [ADR 0132](../adr/0132-go-source-golden-evaluation.md): independent RED/GREEN, direct/Make routing and no-Python scoring; all 32 hosted checks passed on `a34b046`. Merge `b33ab2f` has the same tree. |
| [ ] | `eval/runners/mock.sh` | `eval run mock` / `evalcmd` | Runner boundary: retain deterministic pass/fail/cheat behavior and golden-evaluation integration. |
| [ ] | `eval/runners/claude.sh` | `eval run claude` / `evalcmd` | Runner boundary: literal workdir/prompt, existing argv, model selection and log behavior; fake CLI qualification. |
| [ ] | `eval/runners/codex.sh` | `eval run codex` / `evalcmd` | Same runner boundary, with Codex's existing invocation contract. |
| [ ] | `eval/runners/opencode.sh` | `eval run opencode` / `evalcmd` | Same runner boundary, with OpenCode's existing invocation contract. |
| [ ] | `eval/runners/example-harness.sh` | `eval run example-harness` / `evalcmd` | Preserve the deliberately unwired example and its refusal; do not select a model. |
| [ ] | `scripts/harness-structural-eval.sh` | `eval structural` / `evalcmd` | Structural-eval boundary: three adapter probes, real role-denial controls, Make and pack CI consumers. |
| [ ] | `scripts/adversarial-review.sh` | `review` / `adversarialreviewcmd` | Review-execution boundary: existing provider/configuration semantics, fake HTTP qualification and the review workflow's direct-script consumer. |
| [ ] | `scripts/factory-review-lane.sh` | `review-lane` / `reviewlanecmd` | Review administration: opt-in enable/disable/status behavior, workflow ownership, terminal input and historical fixtures. |
| [ ] | `scripts/factory-migrate-config.sh` | `migrate-config` / `migrateconfigcmd` | Configuration migration: dry-run/apply, preserved data and failure behavior, historical fixtures. |
| [ ] | `scripts/pre-push-check.sh` | `check` / `checkcmd` | Check orchestration: eight-stage ordering, real stdin, caller role, Git ranges and local-hook operand compatibility. |
| [ ] | `scripts/factory-budget.sh` | `budget` / `configuredcmd` | Controller boundary: preserve configuration, models, state and process ownership while removing Python dispatch. |
| [ ] | `scripts/factory-loop.sh` | `loop` / `configuredcmd` | Controller boundary: preserve checkpoints, budgets, opt-in execution and cancellation while removing Python dispatch. |
| [ ] | `scripts/selftest/run.sh` | `selftest` / `installedproof` | Proof-runner boundary: native public routing plus independent custody/coverage for the broader historical regressions; see the proof-retirement ledger. |

The five evaluation-runner rows have local implementation and independent
qualification under Decision 148 / [ADR 0133](../adr/0133-go-source-evaluation-runners.md).
Twenty-five new criteria cover their source routing, native execution, nested
shutdown, failure reporting and source-bootstrap refusal. The combined local race
run passed 72 criteria; final test inventory/partition qualification passed all
11 selected criteria with 3257 total and 53 source criteria. These five rows stay
open until hosted checks, review responses and merge complete. They are not five
new runtime capabilities: their native owners already existed.

## Remaining sourceable libraries

Reuse the existing private Go bridge protocols. Caller-shell assignment and
operand expansion remain in shell where required; parsing and computation have
one Go owner. Sourcing an inert reader must not launch work. Source-time palette
and budget exports are intentional effects and require independent checks.

| Done | Path | Existing owner / bridge | Remaining compatibility evidence |
|---|---|---|---|
| [ ] | `scripts/lib/config.sh` | `config`; `runtime/shell/readers.sh`, `runtime/shell/config.sh` | Source locator/bootstrap, flat-file and environment precedence, literal export/setter plans, local-hook expansion and caller variable attributes. |
| [ ] | `scripts/lib/roles.sh` | `roles`; `runtime/shell/readers.sh` | Role/tier functions, missing operands, nounset callers and no source-time execution. |
| [ ] | `scripts/lib/events.sh` | `eventlog`, `helpers`; `runtime/shell/helpers.sh` | Best-effort append/retention and gate consumers; logging failure must not change gate decisions. |
| [ ] | `scripts/lib/timing.sh` | `helpers`; `runtime/shell/helpers.sh` | Duration formatting, zero/unknown fallback and sourceable POSIX caller behavior. |
| [ ] | `scripts/lib/color.sh` | `presentation`, `helpers`; `runtime/shell/helpers.sh` | Source-time TTY/NO_COLOR palette, literal action-box output and caller overrides. |
| [ ] | `scripts/lib/hookspath.sh` | `githooks`, `helpers`; `runtime/shell/helpers.sh` | Git's actual configured/executable hook classification and literal returned path. |
| [ ] | `scripts/lib/budget-config.sh` | `config`, `roles`, `helpers`; `runtime/shell/helpers.sh` | Source-time complete settings plan, model-tier resolution and all-or-nothing caller-variable validation. |

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
