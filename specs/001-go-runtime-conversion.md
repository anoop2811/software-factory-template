# Go Runtime Conversion

**Status:** Draft
**Author(s):** Anoop Gopalakrishnan (direction); Codex (draft)
**Date:** 2026-09-06
**Story/Ticket:** Decision 48 in [DECISION_LOG.md](../docs/DECISION_LOG.md); follows [merged PR #72](https://github.com/anoop2811/software-factory-template/pull/72)
**Sprint/Cycle:** Staged migration; scheduling follows spec review

Format source: [ai-craft SPEC_TEMPLATE.md](https://github.com/anoop2811/ai-craft/blob/3d6c1bbb8f84116e34519382c826030324a06e77/SPEC_TEMPLATE.md), read 2026-09-06.
This draft follows its twelve sections and WHAT/WHY emphasis. Design choices
below are proposals for human review, not approval to start implementation.
Specifications in this repository use `specs/NNN-descriptive-name.md`.

Compatibility baseline: factory commit `76952eaa63aebd1ecd282f5ab51dd7c3627cb497`
(the merged loop feature and review corrections), plus installed repositories
from release `v0.1.6` and older recognized installation/configuration formats.
The baseline is an identified source revision, not a claim that every historical
installation has been tested. Compatibility evidence is a required deliverable.

---

## 1. Overview

Convert factory-owned execution, configuration, enforcement, reporting and
installation logic into a shared Go runtime while preserving how existing users,
CI jobs and Codex, Claude Code and OpenCode invoke the factory. Each migration
stage includes compatibility evidence, recoverable upgrades and cleanup of the
logic it replaces, so adoption does not leave two permanent implementations or
require developers to repair their repositories by hand.

## 2. Problem

The factory currently combines Bash entry points and controls with Python budget
and loop controllers. Configuration, process supervision, filesystem state and
release delivery cross language boundaries, making their shared contracts harder
to maintain as the factory grows. A language change must not discard the safety
behavior established by past fixes or change what an adopter pays for.

For example, an adopter has customized hooks, a non-Go application, an existing
budget ledger and a resumable manual loop. They upgrade the factory. Their old
hook paths must still enforce the same policies, their remaining budget must not
reset, their checkpoint must remain inspectable, and obsolete factory code must
be removed without deleting their customizations. A failed download or interrupted
upgrade must leave a usable installation or an explicit recoverable state.

The goal is a simpler maintained runtime and predictable delivery. Faster model
responses, lower token usage and native Windows support are not implied by Go.

## 3. User Stories

### Story 1: Keep existing integrations working

**As a** developer with an installed factory
**I want** my commands, configuration, hooks and reports to retain their contracts
**So that** an internal language change does not break my daily workflow or CI.

### Story 2: Preserve execution safety and spending choices

**As a** user of Codex, Claude Code or OpenCode
**I want** the same budgets, role boundaries and loop recovery behavior
**So that** conversion cannot launch extra work, weaken gates or lose ownership.

### Story 3: Upgrade and recover safely

**As a** repository maintainer
**I want** a reviewable migration preview and recoverable installation changes
**So that** interruption or incompatibility does not damage my repository.

### Story 4: Remove obsolete factory code safely

**As a** maintainer and adopter
**I want** superseded implementations and references cleaned up during conversion
**So that** maintenance becomes simpler without deleting my custom code or history.

### Story 5: Install a predictable runtime

**As a** developer on a supported machine or offline CI runner
**I want** a release-matched runtime that does not require a Go or Python installation
**So that** I can use factory capabilities independently of my application's stack.

### Story 6: Prove each conversion stage before switching

**As a** factory reviewer
**I want** independent compatibility and failure evidence for every migrated surface
**So that** a smaller codebase does not come at the cost of weaker enforcement.

## 4. Acceptance Criteria

### Story 1: Keep existing integrations working

**AC 1.1: Existing entry points**

> Given a baseline fixture invoking the top-level CLI, a direct script, a generated native hook, or a sourced shell library
> When the corresponding Go migration stage is installed
> Then arguments, working-directory behavior, environment, input/output channels, side effects and exit statuses satisfy the recorded compatibility contract
> And existing paths remain usable without editing caller configuration.

**AC 1.2: Configuration semantics and inert data**

> Given conflicting environment, factory.yaml and legacy factory.config values, including empty values, quotes, comments and shell-looking text
> When either runtime resolves configuration or model roles
> Then precedence, defaults, literal values and standard/economy routing match the baseline
> And data never becomes shell code, unsupported native model mappings remain explicit, and reviewer quality is not lowered.

**AC 1.3: Stored state and previous evidence**

> Given baseline budget records, event logs and both fresh and stale loop checkpoints
> When the Go runtime reports status or resumes an eligible manual checkpoint
> Then identities, counters, unknown usage, reserved time, privacy permissions and outcome meaning are preserved
> And equivalent fingerprints remain comparable, stale evidence stays stale, and reading state does not reset or rewrite it
> And an upgrade that legitimately changes fingerprinted runtime/policy content makes prior evidence stale; conversion never rebases a checkpoint to manufacture resumability
> And recovery status/report remains readable even if unrelated current execution configuration is invalid.

**AC 1.4: Non-Go adopters and extension points**

> Given Go, TypeScript and Java adopters, including Gradle, Maven, multiple packs and customized local hooks
> When they upgrade and invoke their checks
> Then their pack selection, application build files, check commands and extension arguments remain intact
> And the factory's implementation language does not install the Go application pack or impose Go tooling on their product.

### Story 2: Preserve execution safety and spending choices

**AC 2.1: No new paid or background execution**

> Given default-disabled budgets/loops, a manual loop, a report, or an upgrade preview
> When the command runs through any of the three adapters
> Then no model is invoked, no background agent starts, and no model credentials are newly required
> And an enabled bounded loop charges implementer and reviewer invocations to the same existing limits without automatic retries or limit increases.

**AC 2.2: Mixed runtimes and process ownership**

> Given an old controller, Go controller, active reservation, stale active checkpoint or unconfirmed preflight/check/native child in the same repository
> When a second controller, migration or rollback attempts conflicting work
> Then it cannot overlap the owned work or mutate its runtime underneath it
> And a dead parent, empty pre-admission ledger or expired timeout alone does not establish child exit
> And a baseline process paused before admission cannot resume across destructive cleanup without participating in the transition barrier; unbridged installations defer activation/cleanup until explicit quiescence is established.

**AC 2.3: Harmless preflight rejection and uncertain cleanup**

> Given a missing CLI or required flag with readable accounting and no owned process
> When bounded execution stops and a new manual task is requested
> Then the manual check can run without editing a falsely uncertain checkpoint
> But malformed probe responses, signal permission failures, reap errors and unknown exit retain the actual owned PID and block conflicting work when exit is unconfirmed.

**AC 2.4: Policy and time boundaries**

> Given protected/test paths, ignored native policy files, a configured POSIX extended-regex test pattern and a finite deadline
> When implementation, review, deterministic checks or lock contention occur
> Then policy mutations and stale evidence stop the loop, regex behavior matches existing hooks, and no model starts after its admission deadline
> And process-group cleanup remains bounded, independent review stays separate, and terminal bounded runs do not automatically restart on resume.

### Story 3: Upgrade and recover safely

**AC 3.1: Read-only preview**

> Given an installed repository and an available local target source/artifact set
> When `factory upgrade --dry-run --source PATH` is requested
> Then it lists additions, replacements, retained compatibility shims, removals, preserved customizations, conflicts and rollback readiness
> And it changes no project/cache/state files, launches no checks or models, and reports unavailable prerequisites without downloading them.

**AC 3.2: Interrupted or failed application**

> Given a migration interrupted at any artifact-verification, staging, activation or cleanup boundary, including disk-full and permission failures
> When the next factory command starts
> Then it uses a complete compatible installation or explains how to recover before running enforcement or model work
> And retry is idempotent, no partial installation claims success, and no new model call is used as an installation test.

**AC 3.3: Explicit rollback**

> Given a retained recovery record and no active or uncertain owned work
> When `factory upgrade --rollback MIGRATION_ID` is requested
> Then only unchanged migration-owned installation changes return to the previous compatible version
> And later user edits are preserved as conflicts, consumed budget/history is not rewound, and rollback is refused if the old runtime cannot safely read current state
> And installed-version metadata names the prior usable installation until mandatory activation checks complete, rather than reporting success before those checks run.

**AC 3.4: Unknown and customized installations**

> Given missing ownership metadata, an unrecognized prior version or a customized factory-owned path
> When conversion assesses a replacement or deletion
> Then uncertain ownership is reported and the existing file is preserved
> And activation stops if that unresolved conflict prevents a complete compatible installation; it does not claim full conversion or silently overwrite the file.

### Story 4: Remove obsolete factory code safely

**AC 4.1: Proved obsolete files**

> Given an obsolete path proven to belong to the prior factory release and matching its recorded content, type and mode
> When its replacement passes acceptance and migration commits successfully
> Then the obsolete implementation is removed from the active installation and its references are updated
> And any required public compatibility path remains as a thin adapter with one implementation of the behavior.

**AC 4.2: Customizations, path traversal and repeated cleanup**

> Given modified files, untracked files, symlink ancestors, hard links, unsafe archive entries, paths outside the repository, unusual filenames or misleading ownership records
> When cleanup and then a second identical cleanup are requested
> Then only validated, unchanged, migration-owned obsolete entries are removed
> And no user content or referent outside the installation is touched, a changed-since-preview path becomes a conflict, and the second pass has no further removals.

**AC 4.3: Complete conversion inventory**

> Given a stage proposed for completion
> When its source and adopter installation are audited
> Then every baseline owned asset in that stage is mapped to converted Go behavior, a justified retained adapter/declarative asset, or evidence-backed removal
> And stale runtime dispatch, duplicate business logic, retired dependencies, generated references and CI/install copy lists are absent.

### Story 5: Install a predictable runtime

**AC 5.1: Supported artifacts and offline operation**

> Given each supported operating-system/architecture target, an authentic release-matched artifact set and no Go/Python executable on PATH
> When installation and shipped local factory commands run
> Then the migrated behavior runs without compiling or downloading a language runtime
> And normal commands make no network requests beyond explicitly requested existing native/network operations; a pre-provisioned complete bundle supports offline installation and use.

**AC 5.2: Invalid or unavailable artifacts**

> Given a wrong architecture, mismatched source/runtime version, corrupt artifact, failed authenticity verification or unavailable release
> When installation or upgrade is attempted
> Then verification is refused with a useful diagnostic before any candidate code executes, including version probes, staged smoke tests or migration helpers, and before changing the working installation
> And an executable sentinel in the rejected artifact does not run
> And there is no silent fallback to main, an arbitrary PATH binary, an older runtime or a second paid invocation.

**AC 5.3: Existing bootstrap consent**

> Given the existing installer invocation without init or upgrade
> When it fetches a pinned template
> Then it does not execute downloaded code or modify the user's project
> And explicit init/upgrade still targets only the selected repository, honors supported ref/source options and does not commit, push or merge changes.

**AC 5.4: Containment at every migration phase**

> Given traversal or absolute archive entries, symlink ancestors, hard links, or a path changed since preview
> When extraction, staging, replacement, activation, rollback or cleanup assesses a write
> Then only validated migration-owned paths inside explicitly approved installation/staging/recovery roots may change
> And unsafe entries are rejected before their mutation, no outside referent changes, and each phase has a negative fixture demonstrating that refusal.

### Story 6: Prove each conversion stage before switching

**AC 6.1: Differential behavior and negative controls**

> Given isolated old/new fixtures with identical inputs and fake native CLIs
> When the compatibility matrix runs
> Then all specified outputs, side effects, permissions and statuses match after only declared nondeterministic-field normalization
> And every safety-critical acceptance case is seen failing under its targeted defect before it passes; two runtimes agreeing on a defect is insufficient evidence.

**AC 6.2: Non-vacuous installation and enforcement checks**

> Given converted installation code and an explicit list of required assets and registered hooks
> When a required asset is omitted or a hook is bypassed in an isolated negative fixture
> Then installation/manifest validation and behavioral gate proofs fail
> And tests do not pass merely because shell-source greps no longer discover any copy statements or inline implementation text.

**AC 6.3: Honest completion and release evidence**

> Given a migration stage with unfinished compatibility, recovery or cleanup work
> When status is recorded
> Then the stage is incomplete, its remaining work is explicit, and no language-based cost-saving claim is made
> And final conversion requires packaged-artifact tests, all three adapter contracts and the full supported-platform matrix; live paid evidence is separately labeled and requires opt-in.

**AC 6.4: Outside-in TDD for every vertical slice**

> Given an approved user-facing acceptance criterion and its independent test author
> When work starts on a migration slice
> Then a Ginkgo v2 acceptance scenario using Gomega first fails through the real compiled CLI or supported hook/adapter boundary for the intended missing behavior
> And focused lower-level Ginkgo/Gomega specs drive only the collaborating behavior needed by that scenario, with RED recorded before implementation, then GREEN, then refactor while tests stay green
> And completion evidence links the AC ID, failing command/output, passing command/output and regression suite; writing tests after a completed implementation does not satisfy the sequence.

**AC 6.5: Cobra preserves the existing command contract**

> Given the approved CLI compatibility matrix, including unknown commands, invalid flags, help, aliases, positional arguments, --flag=value forms and piped JSON output
> When the Go CLI routes them through Cobra
> Then baseline exit codes, usage/error channels, argument forwarding and output contracts are preserved
> And framework defaults do not introduce unsolicited usage banners, suggestions, completion commands, flag reinterpretation or alternate configuration precedence.

## 5. Functional Requirements

| ID | Requirement | Priority |
|---|---|---|
| FR-001 | The conversion MUST inventory all factory-owned executables, sourced libraries, hook registrations, generated adapters, templates, pack helpers, installation assets and relevant CI/test tooling before a stage replaces them. | MUST |
| FR-002 | Existing CLI names, aliases, script paths, flags, arguments, environment contracts, input/output streams and exit statuses MUST remain compatible; new migration flags MUST be additive. | MUST |
| FR-003 | Shell libraries used by shipped hooks or documented extension points MUST remain sourceable with their existing functions and effects; converted logic MUST have one canonical implementation behind those adapters. | MUST |
| FR-004 | Configuration resolution MUST preserve literal flat-format semantics, legacy fallback, explicit environment precedence, role injection and standard/economy model routing. No parser may evaluate configuration as code. | MUST |
| FR-005 | Existing JSON/event/state contracts MUST retain names, types, null/unknown distinctions, units, required fields and outcome meaning. Timestamp/UUID variability MUST NOT hide semantic differences in tests. | MUST |
| FR-006 | Existing state and fingerprints MUST remain readable/comparable across the transition. A schema change MUST be separately specified with forward-read, downgrade and failure behavior before it ships. | MUST |
| FR-007 | Budgets, defaults, native permissions, opt-in controls, finite retries, independent review and recovery gates MUST preserve [BUDGETS.md](../docs/BUDGETS.md) and [LOOPS.md](../docs/LOOPS.md). | MUST |
| FR-008 | Go and legacy controllers MUST share an interoperable exclusion/ownership boundary covering preflight through final publication. Lock files/inodes MUST NOT be removed/replaced during coexistence; unbridged legacy work requires explicit quiescence before destructive changes. | MUST |
| FR-009 | Process supervision MUST retain group ownership and unknown usage until exit is established, even across preflight, parsing, signaling, ledger publication and wait failures. | MUST |
| FR-010 | Test/protected-path rules MUST continue to come from factory.yaml, including POSIX ERE semantics and ignored native policy inputs. Conversion MUST NOT weaken protection to satisfy tests. | MUST |
| FR-011 | Install/upgrade MUST preserve project configuration, custom hooks, code, tests, application build files, specs, ADRs, memory/wiki content, logs, credentials and runtime history. | MUST |
| FR-012 | Upgrade MUST offer the read-only preview and explicit recovery contracts in section 8, preserve a reviewable diff and perform no automatic Git commit/push/merge. | MUST |
| FR-013 | Activation MUST require complete verified assets and compatibility checks; interrupted changes MUST be recoverable, retryable and never reported as success. | MUST |
| FR-014 | Rollback MUST restore installation assets only; it MUST NOT erase later user edits or reset consumed budget, evidence or process ownership. Incompatible rollback MUST refuse safely. | MUST |
| FR-015 | Cleanup MUST use validated prior ownership and unchanged content/type/mode evidence, rechecked at mutation time. Missing evidence or conflicts MUST preserve the path. | MUST |
| FR-016 | Extraction, staging, replacement, activation, rollback and cleanup MUST validate path/type/ownership and remain within explicitly approved installation/staging/recovery roots. They MUST NOT follow unsafe links, mutate outside referents, recursively delete unclassified directories, or infer ownership solely from a filename/extension/local untrusted manifest. | MUST |
| FR-017 | Every stage MUST retire superseded active logic, update generated adapters/install manifests/CI/docs and remove unused runtime dependencies. Retained compatibility adapters MUST be listed with a reason and tested. | MUST |
| FR-018 | Recovery copies MUST be private, bounded to migration-owned changes and explicitly listed. They MUST NOT become executable fallback paths or duplicate live controllers; user-requested pruning MUST preserve the supported recovery contract. | MUST |
| FR-019 | Official runtime assets MUST match the selected source revision and be integrity/authenticity checked before any candidate execution, including staged inspection/validation, and before activation. Explicit local-source builds MUST remain supported and labeled as local, not authenticated official releases; source and reproducible build instructions MUST be available. | MUST |
| FR-020 | Runtime resolution MUST be deterministic and release-specific. Normal commands MUST NOT auto-update, download, compile, phone home or silently switch runtimes on error. | MUST |
| FR-021 | Final shipped factory commands and self-checks MUST not require Go or Python at runtime. Explicit native CLIs, Git, shell entrypoints and an adopter's own check tools remain external prerequisites. | MUST |
| FR-022 | Initial supported targets MUST be Linux amd64/arm64 and macOS amd64/arm64, subject to the minimum-OS qualification in Q2. Each advertised target MUST run the packaged acceptance suite. | MUST |
| FR-023 | Factory development tooling MUST follow the Go pack's Ginkgo/Gomega and quality conventions without changing an adopter's language-pack selection. Tool versions MUST be checked against authoritative releases before pinning. | MUST |
| FR-024 | Generated adapters and their enforcement configuration MUST remain drift-checked for Codex, Claude Code and OpenCode; no parallel per-harness policy implementations may be introduced. | MUST |
| FR-025 | Optional pack fixtures and paid review/eval lanes MUST retain their explicit availability/credential gating and truthful skip reporting. | MUST |
| FR-026 | Every FR MUST trace to an acceptance criterion and independent evidence before completion; negative controls and a complete asset inventory MUST prevent vacuous parity or cleanup claims. | MUST |
| FR-027 | Historical command paths MUST remain supported through this migration. Their eventual removal, if wanted, MUST be a separate compatibility decision, not incidental cleanup. | MUST |
| FR-028 | A baseline behavior shown to be unsafe MUST be recorded as an explicit reviewed correction with a regression case; neither silent incompatibility nor preservation of a known defect is acceptable. | MUST |
| FR-029 | The Go CLI MUST use Cobra for command/flag routing. Cobra defaults MUST be configured to meet the existing public contract; Cobra adoption MUST NOT replace factory configuration semantics or introduce unrequested commands. | MUST |
| FR-030 | Every migration slice MUST use outside-in TDD: a failing user-visible Ginkgo v2/Gomega acceptance scenario, focused red/green collaborator specs as needed, then refactoring with the relevant suite green. Required evidence MUST record that order and its linked AC. | MUST |
| FR-031 | All new Go behavioral tests MUST use Ginkgo v2 and Gomega. The stdlib testing package MUST appear only in RunSpecs suite bootstraps; test/spec authors and implementation authors MUST remain separate under the factory rules. | MUST |

Traceability: Story 1 / AC 1.x -> FR-001..006, 010..011, 024, 027;
Story 2 / AC 2.x -> FR-007..010; Story 3 / AC 3.x -> FR-011..016, 018;
Story 4 / AC 4.x -> FR-001, 003, 015..018, 027;
Story 5 / AC 5.x -> FR-019..023; Story 6 / AC 6.x -> FR-023..026, 028..031.

## 6. Non-Functional Requirements

These are proposed acceptance thresholds, not measured improvements.

| ID | Requirement | Threshold | How to Verify |
|---|---|---|---|
| NFR-001 | Behavioral compatibility | 100% of inventoried required cases pass; zero unreviewed contract differences | Differential subprocess fixtures and explicit exception register |
| NFR-002 | Data preservation | Zero lost/rewound user bytes, history entries, budget reservations or customized modes across failure fixtures | Before/after content, mode and state comparison at each injected boundary |
| NFR-003 | Read-only and cost behavior | Zero filesystem writes/model calls for specified read-only paths; zero added model invocations for equivalent tasks | Filesystem snapshots, fake CLI invocation counts and blocked-network fixtures |
| NFR-004 | Timeout and resource bounds | Preserve existing configured execution ceilings, bounded cleanup waits and output/state limits; zero unbounded waits | Fault injection for full pipes, signals, unreaped children, lock contention and oversized state |
| NFR-005 | Local command responsiveness | Go p95 wall time <= 1.25 times baseline for help, config resolution, budget plan/report and no-op hooks | At least 30 warm and 10 cold runs per case on the same runner; record size, OS and architecture; no live model timing |
| NFR-006 | Distribution coverage | 4/4 advertised OS/architecture pairs pass packaged tests before release | Artifact-based CI with no Go/Python available to the executing factory |
| NFR-007 | Runtime maintenance | One active canonical implementation per migrated behavior; zero undeclared legacy dependencies | Complete conversion inventory plus static references and runtime execution traces |
| NFR-008 | Private recovery/state | Owner-only access no broader than existing state permissions; zero persisted prompts/secrets or full native outputs added by migration | Permission/symlink tests and seeded sensitive-marker scans |
| NFR-009 | Repeatability | Second successful apply/cleanup produces zero additional changes; repeated preview changes nothing | Same-input fixture reruns with content/type/mode comparisons |
| NFR-010 | Outside-in delivery evidence | 100% of completed migration slices have linked acceptance RED -> collaborator RED/GREEN as needed -> acceptance GREEN -> refactor evidence | Per-slice command/output record and independent review; no after-the-fact reconstruction |

## 7. Data Model

These are domain concepts, not prescribed storage schemas.

- **Compatibility surface:** stable identity, entrypoint/function/format, baseline revision, expected observable behavior, owning migration stage, linked ACs and evidence.
- **Installation asset:** repository-relative identity, prior release origin, observed and expected content/type/mode, ownership confidence, replacement or retention reason. A matching filename alone is not ownership.
- **Runtime release:** immutable source revision, target platform, artifact identity, verification evidence and supported state/adapter contracts.
- **Migration plan:** source/target installation identity, explicitly approved installation/staging/recovery roots, assessed assets, proposed actions, conflicts, required prerequisites and rollback eligibility. Preview is transient and read-only.
- **Migration record:** applied plan identity, before/after asset evidence, recovery assets, progress/failure state and explicit next action. It contains installation metadata, not copied user runtime history.
- **Budget reservation and loop checkpoint:** existing domain records remain governed by BUDGETS/LOOPS; migration must preserve their identity, elapsed allowance, ownership and evidence validity.

One migration plan contains many asset actions. A completed migration identifies
one compatible runtime release and its retained adapters. Recovery references
only that migration's owned changes; it is not a repository snapshot reset.

## 8. API Contract

This is a local CLI, hook, sourced-library and persisted-format compatibility
boundary. Cobra is required for Go command/flag routing; its defaults are not the
authority for compatibility. No new HTTP service, daemon or public Go SDK is required.

| Surface | Contract |
|---|---|
| `factory` with no args; `help`, `-h`, `--help` | Retain help success and discoverability; update implementation descriptions truthfully. Unknown commands retain exit 2 and stderr diagnostics. |
| `init`, `upgrade`, `doctor`, `check`, `selftest`, `review-lane`, `metrics`, `report`, `budget`, `loop`, `migrate-config` | Preserve baseline invocation/options, behavior, pipe/TTY handling and statuses. Inventory their full option matrix before moving each command. |
| `scripts/factory-*.sh`, `scripts/hooks/*.sh`, pre-push, citation, sync and eval entrypoints; pack helpers | Preserve existing direct callers and hook payload protocols. Adapters may dispatch to Go; they must not duplicate enforcement logic. |
| Sourced `scripts/lib/*.sh` | Retain supported function names, arguments, output and caller-shell environment effects. A subprocess exit is not a substitute for sourced function behavior. |
| Budget `plan/run/report`, loop `plan/run/status/resume`, metrics JSON, reports/events | Preserve existing schema/version semantics, stdout/stderr separation, null usage and nonzero failure behavior; no extra banners contaminate machine output. |
| Native harness invocation | Preserve literal argument boundaries, model/role mapping, permission/trust checks, stdin/stream handling and cost normalization. No new native CLI flag is assumed. |
| `factory upgrade --dry-run --source PATH` (additive) | Local read-only preview; exit 0 when applicable without conflicts, 2 for invalid/incompatible/conflicting input, 1 for assessment I/O failure. No activation, download or cleanup. |
| `factory upgrade --rollback MIGRATION_ID` (additive) | Explicit installation-only rollback; 0 on complete success, 2 for invalid/ineligible/conflicting recovery, 1 for execution failure requiring recovery. Cannot combine with apply/preview/ref/source selection. |

The inventory must characterize the following non-obvious baseline contracts,
with fixtures rather than assumptions:

| Boundary | Required characterization | Baseline source |
|---|---|---|
| Flat configuration | First matching key, quoted hashes/comments, absent versus empty, get versus has, unrelated-line preservation, legacy allowlist; no general-YAML reinterpretation | scripts/lib/config.sh:5, scripts/lib/config.sh:36, scripts/lib/config.sh:81, scripts/lib/config.sh:241 |
| Caller-shell state | Source-time exports and function effects; retain supported POSIX versus Bash entrypoints; never use unsafe eval on values returned by Go | scripts/lib/config.sh:125, scripts/lib/budget-config.sh:19, scripts/lib/timing.sh:9 |
| Local hook grammar | Comma/whitespace entries, attached flags, literal argv, captured output retaining the hook's own exit code | scripts/lib/config.sh:130, scripts/pre-push-check.sh:126 |
| Budget output | JSON Lines may contain multiple metadata records; normal successful output also displays the answer; no extra banner in machine output | scripts/lib/budget.py:289, scripts/lib/budget.py:590 |
| Recovery inspection | Budget report / loop status precede unrelated current configuration validation | scripts/lib/budget.py:615, scripts/lib/loop.py:566 |
| Checkpoint identity | Sorted-key JSON bytes, ASCII escaping, separators and numeric representation; exact content/mode/policy inputs | scripts/lib/loop.py:24, scripts/lib/loop.py:305 |
| Lock/storage behavior | budget.lock blocking transaction lock, loops.lock nonblocking whole-controller lock, same native lock namespace/inodes, private atomic durable state | scripts/lib/budget.py:194, scripts/lib/loop.py:271 |
| Optional presentation/events | Best-effort event/timing failures remain nonfatal; overridden log paths, TSV retention, NO_COLOR and TTY/CI/browser behavior remain compatible | scripts/lib/events.sh:3, scripts/lib/color.sh:9, scripts/factory-metrics.sh:286 |

These source citations refer to the identified baseline; subsequent conversion
must replace them with stable acceptance references before the files disappear.

Existing upgrade apply statuses remain the baseline contract. New migration
control output must name preserved conflicts and recovery steps without exposing
secrets. No machine-readable migration schema is promised until explicitly
specified; existing JSON contracts are fully in scope.

Backward compatibility is observable equivalence, not identical help prose or
nondeterministic UUIDs/timestamps. For consumed machine output and hook protocols,
field names/types and exact tokens used by consumers are contractual. Any other
normalization must be enumerated in the compatibility evidence before testing.

## 9. Out of Scope

- Implementing this draft, publishing binaries, upgrading adopters or changing toolchain pins in the spec PR.
- New graph/RAG/memory/MCP features, model routing features or changes to the eleven-item capability roadmap.
- Native Windows support or a claim that all shell/platform dependencies disappear.
- Rewriting product code, user-authored hooks, language pack policies, agent instructions, specs or wiki content into Go.
- Removing stable compatibility paths, resetting state, clearing uncertain processes, changing budgets, or claiming strict dollar ceilings.
- A permanent user-facing choice between two full runtime implementations, a hosted control plane, telemetry, background updates or paid migration validation.

## 10. Open Questions

Owners and dates are proposed review checkpoints; unresolved entries block the
named stage, not completion of this draft.

| # | Question | Owner | Target Date | Resolution |
|---|---|---|---|---|
| Q1 | [NEEDS CLARIFICATION] Which exact older releases must be included beyond v0.1.6 and the merged baseline, based on adopter evidence? | Anoop | 2026-09-13, before compatibility inventory approval | Pending; recognized older formats remain preserved, unknown installations must not be destructively upgraded. |
| Q2 | [NEEDS CLARIFICATION] What minimum Linux distribution/libc and macOS versions can be supported on the four proposed targets, and what runners prove them? | Migration maintainer; Anoop approves | 2026-09-13, before artifact design | Pending; match existing supported adopter environments before claiming portability. |
| Q3 | [NEEDS CLARIFICATION] Which release authenticity mechanism and trust source can run in both connected and pre-provisioned offline installation? | Release maintainer; Anoop approves | 2026-09-13, before first binary distribution | Pending; checksums alone must not be described as publisher authentication. |
| Q4 | [NEEDS CLARIFICATION] How many previous installation versions and how much recovery storage should be retained, and what explicit pruning interface is appropriate? | Anoop | 2026-09-13, before upgrade/recovery implementation | Pending; retain at least the immediately preceding recoverable installation until explicit pruning; no silent removal. |
| Q5 | [NEEDS CLARIFICATION] What evidence period and adopter pilot is required before making Go the default and removing active legacy implementations? | Anoop | 2026-09-13, before default cutover | Pending; all acceptance and cleanup gates are mandatory regardless of calendar duration. |

**Known discrepancy EX-001 (not silently part of parity):** an independent
baseline fixture observed legacy configuration overwriting a caller value during
`factory_config_export`, despite the documented caller > YAML > legacy rule
(scripts/lib/config.sh:181, scripts/lib/config.sh:194,
scripts/lib/config.sh:217). G0 must capture both the observed result and intended
contract. Anoop must approve either a separately tested prerequisite correction
in both baselines or a specifically documented migration correction before G1;
until then the affected parity case is blocked. This draft changes no runtime
behavior and does not require preserving this defect as the desired outcome.
Owner: Anoop; target: 2026-09-13, before G1 approval.

## 11. Decision Record

The following are draft architecture decisions associated with Decision 48.
Detailed package layouts and implementation interfaces belong in subsequent
design records, after these compatibility contracts are reviewed.

### Decision 1: One Go runtime, stable external adapters

**Context:** Both factory logic and users' integrations currently span scripts.

**Decision:** Propose converting owned runtime behavior to Go and retaining thin
bootstrap, command, hook and sourced-library compatibility adapters. Keep packs,
policies, templates and user extensions in their natural declarative formats.

**Alternatives considered:** A full path-removing rewrite would break callers;
keeping all Python/Bash logic indefinitely would retain duplicate maintenance;
moving only loops would leave the wider conversion/cleanup request unfinished.

**Consequences:** A compiled artifact becomes part of release delivery, explicitly
revisiting Decision 14's no-binary choice. Source remains inspectable. A smaller
runtime dependency set requires more disciplined release and platform testing.

### Decision 2: Staged conversion with cleanup in each slice

**Context:** A single rewrite would make regressions difficult to isolate, while
an endless transition would never deliver the requested simplification.

**Decision:** Propose these stages with release-specific routing and no automatic
runtime fallback. Each stage is complete only after its outside-in TDD, parity, failure, delivery
and cleanup evidence passes. Stage order does not mark roadmap features complete.

| Stage | Scope | Required exit evidence | Implementation progress |
|---|---|---|---|
| G0 | Baseline surface/asset inventory, compatibility fixtures, target and recovery design | Reviewed contracts, negative controls, owned/retained/removal map; open questions resolved for next stage | 0% |
| G1 | Cobra CLI, runtime packaging, local configuration/role resolution, compatibility entrypoints | Packaged artifact validation, inert config parity, deterministic version selection, sourceable adapters and missing-artifact refusal | 0% |
| G2 | Shared process supervision, budgets, native adapters and loops | Three harnesses, mixed-runtime exclusion, state/fingerprint parity, deadline/cleanup failure coverage; retire replaced Python runtime logic | 0% |
| G3 | Factory gates, sync, doctor, check/selftest, reports/metrics and eval/review orchestration | Actual gate break/fix proofs, adapter drift, output parity, optional/paid gating; retire corresponding duplicate shell logic | 0% |
| G4 | Init/upgrade/config migration and installation lifecycle | Legacy/current adopter fixtures, verified artifacts, preview, interrupted apply, rollback, customization-safe cleanup and explicit manifest proofs | 0% |
| G5 | Default cutover and final dependency/reference audit | All stages pass on packaged targets; complete adopter upgrade/recovery rehearsal; zero unclassified assets or retired active implementations | 0% |

Artifact delivery/recovery needed for an earlier stage must be available before
that stage reaches adopters; G4 expands and completes the lifecycle conversion,
it is not permission to ship G1-G3 without safe upgrades. A compatible transition
bridge must cover baseline entrypoints before concurrent destructive conversion;
otherwise a documented quiescence prerequisite prevents activation and cleanup.
Do not assume a new Go-only lock can stop an already-running legacy process. The final source review
must account for runtime-adjacent shell/embedded Python outside `scripts/lib`,
including pack helpers, generated assets, tests and CI. Historical fixtures may
remain test-only with an explicit rationale; they are not installed fallback code.

### Decision 3: Preserve state and prove ownership before cleanup

**Context:** Migration can destroy safety by changing a lock, digest format or
ledger field even when ordinary command output looks identical. Deleting files
by directory or extension can destroy adopter customizations.

**Decision:** Propose preserving current state semantics and lock interoperability
through the initial conversion, with no opportunistic schema change. Cross-runtime
fixtures must include exact legacy digest vectors (floats, Unicode, nulls and
modes) and both lock acquisition orders. An upgrade that changes governed inputs
can invalidate evidence legitimately; preserving compatibility never authorizes
changing stored fingerprints to make it fresh. Replacements
and removals require a known source version and rechecked owned-asset evidence;
recovery restores installation changes without restoring old runtime history.

**Alternatives considered:** Resetting state would erase budget/ownership evidence;
blanket removal would erase custom files; trusting a local manifest without
validating origins and paths would turn metadata into deletion authority.

**Consequences:** Some customized or unknown installations require manual conflict
resolution. Compatibility shims may remain permanently where they are public
contracts; obsolete implementations must not remain active behind them.

### Decision 4: Reuse behavior contracts, independently prove Go conformance

**Context:** Existing selftests include implementation-text assertions as well as
real subprocess fixtures. Text checks can become vacuous after conversion.

**Decision:** Propose independent black-box compatibility and fault-injection
coverage using outside-in TDD with the Go pack's Ginkgo v2/Gomega conventions
for all new Go behavioral tests, alongside
baseline fixtures until their behavioral coverage is replaced and demonstrated.
Use explicit asset/registration inventories rather than discovering correctness
solely by grepping shell implementation text.

**Alternatives considered:** Passing only old textual checks could hide missing
assets; replacing all tests with new happy-path tests would lose scar-tissue
regressions; dual-running live harnesses for parity would double spending.

**Consequences:** Generator/evaluator separation remains mandatory. Fake native
CLIs and real process/filesystem boundaries are the default evidence; live model
quality is separate. Published performance/cost claims require measurements.

### Decision 5: Qualify toolchain and process guarantees from authoritative sources

**Context:** Toolchain and platform support move over time; a standard-library
cancellation API alone does not prove descendant cleanup or state compatibility.

**Decision:** The implementation stage must re-check authoritative releases before
pinning its toolchain and development tools. On 2026-09-06, the official
[Go release history](https://go.dev/doc/devel/release) lists Go 1.27.1, released
2026-09-01; this is research provenance, not a pin added by this draft. The
[os/exec contract](https://pkg.go.dev/os/exec#CommandContext) provides cancellation
controls but leaves process-group ownership and factory recovery obligations to
the implementation. No new Codex/Claude/OpenCode API or flag is selected here.

**Alternatives considered:** Pinning from memory risks stale dependencies;
assuming language migration solves process cleanup would reproduce known defects.

**Consequences:** The build/version design must document exact checked versions,
supported targets and reproducible commands before adding dependencies.

### Decision 6: Cobra and outside-in Ginkgo/Gomega TDD are required

**Context:** The user explicitly selected Cobra and outside-in TDD with Ginkgo
and Gomega while reviewing this draft on 2026-09-06.

**Decision:** Use Cobra for the Go command surface and implement vertical slices
from failing external behavior inward. The outer acceptance scenario exercises
the real CLI or existing adapter boundary; lower-level specs introduce only
collaboration needed to make it pass. Refactoring follows green tests. The
independent evaluator owns acceptance criteria and tests, while the implementer
owns production changes. This applies to every stage, including packaging,
upgrade, rollback and cleanup, not only budget/loop domain code.

**Alternatives considered:** A custom parser conflicts with the selected
framework; replacing factory configuration with a general framework integration
risks precedence drift; inside-out implementation followed by acceptance tests
does not satisfy the requested development process.

**Consequences:** Cobra's error/usage/flag defaults require explicit compatibility
coverage. Ginkgo/Gomega remain development dependencies; shipped local self-checks
must be runnable without requiring end users to install Go or Python. No Viper
or additional CLI framework is selected by this decision.

References read 2026-09-06: [Cobra documentation](https://cobra.dev/docs/),
[Ginkgo documentation](https://onsi.github.io/ginkgo/),
[Gomega documentation](https://onsi.github.io/gomega/).
Exact dependency versions must be checked against their official releases when
implementation pins them; this spec creates no go.mod or go.sum.

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|---|---|---|---|
| Conversion completion | 0%; specification only | G0-G5 acceptance, delivery and cleanup complete | Stage evidence linked to AC/FR IDs, independent review and human release decision |
| Required behavior differences | Inventory not yet complete | Zero unreviewed differences | Full baseline/Go compatibility matrix |
| Lost customizations or history | Not measured for conversion | Zero in all migration/recovery failure fixtures | Content/mode/state comparison and interruption injection |
| Duplicate active implementations | Bash/Python runtime split; exact inventory pending | One per converted behavior, zero undeclared leftovers | Asset map, dependency scans and execution traces |
| Runtime language prerequisites | Budget/loop use Python; shell entrypoints remain | No Go/Python needed for shipped factory behavior; retained external prerequisites explicit | Clean packaged-artifact environment tests |
| Supported packaged targets | No Go runtime artifacts yet | 4/4 qualified targets | Per-target release acceptance |
| Local command latency | Baseline not measured | NFR-005 threshold met | Recorded controlled benchmark comparison |
| Outside-in TDD adherence | No Go slices implemented | 100% of completed slices follow AC 6.4 with independent test ownership | Recorded RED/GREEN/refactor evidence linked to AC IDs |
| Conversion-induced model usage | Not applicable yet | Zero extra calls for equivalent workloads | Fake native invocation accounting; paid evidence reported separately |

## Review Checklist

- [ ] Problem and user value accepted by Anoop.
- [ ] Stories and Given/When/Then criteria reviewed, including failure and recovery paths.
- [ ] MUST requirements and measurable thresholds accepted; open questions resolved before their dependent stages.
- [ ] Compatibility inventory covers direct scripts, sourced functions, generated hooks, JSON/state, packs and older adopters.
- [ ] Cleanup preserves customizations, retains required compatibility paths and cannot reset runtime history.
- [ ] Artifact trust, platform qualification and interrupted-install recovery are approved before distribution.
- [ ] Spec describes WHAT/WHY; implementation structure remains in later design records.
- [x] Independent correctness, security and acceptance review completed; two findings closed in AC 5.2/FR-019 and AC 5.4/FR-016 (2026-09-06).
- [ ] Cobra compatibility and outside-in Ginkgo/Gomega TDD are explicit stage gates.
- [ ] Human approval recorded before implementation begins; no draft percentage treated as shipped functionality.
