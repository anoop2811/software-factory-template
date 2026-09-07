# Baseline compatibility inventory

Decision 49 (`docs/DECISION_LOG.md:1800`) starts characterization before production
routing changes. The governing conversion contract is
[the Go runtime specification](../../specs/001-go-runtime-conversion.md).

[baseline-assets.json](baseline-assets.json) inventories **160 tracked assets** at
immutable behavior baseline `76952eaa63aebd1ecd282f5ab51dd7c3627cb497`.
It covers the full Git tree, including metadata, native adapters, packs, examples,
tests, CI, documentation and knowledge content. It deliberately excludes later
specification and development files: those are not part of this behavior baseline.
Release v0.1.6 and other supported adopter fixtures are additional compatibility
inputs; this manifest does not claim to inventory or test their installations.

## Reading the manifest

Each entry contains its repository-relative path, Git mode, SHA-256 of its exact
Git blob bytes, Git blob identity, byte count, classification, proposed stage,
path disposition, legacy-logic action and reason. `CLAUDE.md` is a Git symlink
(mode `120000`); its hash covers the nine-byte link target `AGENTS.md`, not the
referent's contents. There are no submodules in this baseline tree.

`disposition` describes the fate of the path, not whether its present body stays:

- `retain`: keep the data, policy or public path. A retained script may become a
  thin Go adapter; `legacy_logic_action: convert` identifies that planned change.
- `convert`: replace an owned asset with its Go equivalent after acceptance. No
  baseline path needs this label alone: stable paths are retained and the three
  internal Python implementation paths have explicit retirement plans.
- `retire`: remove an internal active implementation only after its replacement,
  ownership, delivery, recovery and cleanup requirements pass.

The other logic actions are `none`, `audit-embedded-helpers`, and
`replace-active-test-execution`. They prevent declarative assets and historical
test evidence from being confused with obsolete product runtime code. A stage is
an intended review/cutover responsibility, not a completion claim. There are 157
retained paths and three proposed internal retirements; **nothing is retired now**.

This is a source inventory, **not an adopter ownership or deletion manifest**.
Matching a source path or this hash does not authorize overwriting customizations,
changing a Git index, deleting backups or resetting runtime history. Installation
ownership, safe paths and current bytes/modes require separate evidence under
the approved migration and recovery contracts.

## Legacy implementation sunset map

| Stage | Owned computation to replace | Paths and evidence retained |
|---|---|---|
| G1 | CLI routing and shared configuration/role computation, subject to EX-001 resolution | `factory`, `scripts/lib/config.sh`, `scripts/lib/roles.sh` remain compatible public/sourceable paths. Configuration remains inert data. |
| G2 | Python budget, native-invocation and loop implementations in `scripts/lib/budget.py`, `scripts/lib/budget_adapters.py`, `scripts/lib/loop.py` | Retire those three internal active files only after Go conformance and safe delivery. Keep `scripts/factory-budget.sh`, `scripts/factory-loop.sh`, `scripts/lib/budget-config.sh` as compatible adapters. Preserve native configuration formats and canonical role instructions. |
| G3 | Gate/check/doctor/sync/report/metrics/eval orchestration, pack dialect-check computation, runtime-adjacent embedded helpers | Keep public scripts/hooks/eval paths and sourceable libraries as thin adapters. Keep host-required plugin code, policy, product-language build files, browser assets and test evidence in appropriate formats. |
| G4 | Init, upgrade and configuration-migration computation | Keep `install.sh`, `scripts/factory-init.sh`, `scripts/factory-upgrade.sh`, `scripts/factory-migrate-config.sh` consent and invocation contracts. Release-specific artifact selection, transactional backup/recovery and predecessor-aware cleanup must exist before any earlier stage reaches adopters. |
| G5 | Final active dependency and reference audit | Retain documentation, licenses, lessons, wiki, specifications and project metadata. Remove only proven obsolete active references/dependencies; retained public wrappers are not incomplete conversion. |

G3 must include embedded Python outside `scripts/lib`: metrics processing and
rendering (`scripts/factory-metrics.sh:121`, `scripts/factory-metrics.sh:268`),
eval output/comparison (`scripts/golden-task-eval.sh:230`), prerequisites
(`scripts/prereq-check.sh:102`), and active acceptance helpers under
`scripts/selftest/`. Keeping historical fixture source is permissible only with
an explicit test-only rationale and exclusion from installed fallback/runtime
discovery. Final shipped self-checks must not silently retain a Python dependency.

## Public surfaces requiring independent characterization

This is the surface inventory and acceptance checklist, not evidence that every
listed behavior has already passed a differential test. Baseline references name
the identified revision; replace disappearing source citations with stable
acceptance references before cleanup.

| Surface | Contract to characterize | Baseline reference |
|---|---|---|
| Top-level CLI | No-argument/help aliases; complete command set; literal argv; unknown-command stderr and exit 2; missing/nonexecutable command refusal; `exec` status/signal behavior | `factory:40`, `factory:56`, `factory:61` |
| Direct executable paths | Existing command, hook, eval and pack entrypoints; subdirectory invocation; environment, stdin/TTY handling, stdout/stderr and exit semantics | Every manifest entry classified as a public or language-pack adapter |
| Sourced libraries | Supported function names, caller-shell exports/effects, failure semantics and POSIX/Bash requirements; no unsafe evaluation of Go output | `scripts/lib/config.sh:125`, `scripts/lib/config.sh:174`, `scripts/lib/budget-config.sh:19` |
| Configuration and role routing | Flat-format parsing, first-key behavior, quotes/hashes, absent/empty/get/has distinctions, allowlisted legacy data, model tiers, explicit role injection | `scripts/lib/config.sh:27`, `scripts/lib/config.sh:94`, `scripts/lib/config.sh:181`; EX-001 remains pending |
| Local hook registration | Whitespace/comma syntax, attached flags, literal arguments and preserved hook output/status | `scripts/lib/config.sh:130`, `scripts/pre-push-check.sh:126` |
| Native adapters | Codex/Claude/OpenCode argv and stdin, role permissions, hook trust, typed probe ownership, stream normalization and unknown cost | `scripts/lib/budget_adapters.py:26`, `scripts/lib/budget_adapters.py:194`, `scripts/lib/budget_adapters.py:260`; `.claude/`, `.codex/`, `.opencode/`, `opencode.json`, `.mcp.json` |
| Budget and loop state | Schema/numeric/null semantics, exact fingerprint bytes, private atomic writes, interoperable lock paths/inodes, reservations and recovery inspection before unrelated config validation | `scripts/lib/budget.py:194`, `scripts/lib/budget.py:563`, `scripts/lib/loop.py:24`, `scripts/lib/loop.py:271`, `scripts/lib/loop.py:606` |
| Process execution | Deadlines after lock contention, process-group cleanup, unknown exit/usage, output limits, no automatic paid retry, ordinary preflight rejection versus uncertain ownership | `scripts/lib/budget.py:484`, `scripts/lib/budget.py:493`, `scripts/lib/budget_adapters.py:110`, `scripts/lib/loop.py:329` |
| Reporting and presentation | Machine-output framing and types, answer visibility, TSV/event retention, optional timing/color/browser behavior, best-effort failures | `scripts/lib/budget.py:289`, `scripts/lib/events.sh:3`, `scripts/lib/color.sh:9`, `scripts/factory-metrics.sh:286` |
| Install/upgrade/bootstrap | Explicit fetch/init/upgrade consent, ref/source selection, preserved identity/configuration, bounded upgrade handoff and nestedness, actual asset completeness | `install.sh:95`, `install.sh:170`, `scripts/factory-upgrade.sh:34`, `scripts/factory-upgrade.sh:175`, `scripts/factory-upgrade.sh:289` |
| Product packs | Go, TypeScript, Java/Gradle/Maven and polyglot behavior; pack test regexes, local hooks and unchanged product build selection | `packs/*/pack.yaml`, pack hook/build/workflow entries; `scripts/selftest/java-build-tool.sh`, `scripts/selftest/maven-quality.sh`, `scripts/selftest/optional-packs.sh` |
| Tests and CI | Behavioral negative controls, registered-hook proofs and explicit asset completeness rather than shell-text grep success; separate optional/paid gating | `scripts/hooks/copy-manifest-check.sh:4`, `scripts/selftest/`, `.github/workflows/`, `eval/` |

## Gates still open

- EX-001 is a recorded discrepancy between observed legacy configuration behavior
  and documented caller precedence. The user's correction/parity choice remains
  pending. The inventory neither resolves it nor changes configuration behavior.
- Q1 (older release coverage), Q2 (minimum platforms), Q3 (artifact trust) and Q5
  (rollout/pilot evidence) remain stage gates. Q4 retention follows the approved
  local gitignored backup policy; it is not implemented by this inventory.
- Native plugin behavior must be characterized as observed. For example,
  `.opencode/plugin/factory-hooks.ts:72` contains an existing fail-open error
  branch. A correction must follow FR-028's explicit review and regression path;
  neither the inventory nor a Go rewrite silently approves different behavior.
- No default runtime switch, release artifact distribution, deletion, backup,
  paid native execution or claim of complete G0 acceptance follows from listing
  the files. Independent completeness/negative-control and behavioral evidence
  must accompany the stage before it can be marked complete.

## Inventory reproduction boundary

The list comes from `git ls-tree -r -z` at the immutable baseline. Modes and Git
object identities come from that tree; byte counts and SHA-256 values come from
`git cat-file blob` for each object. Hashes never use current worktree content.
Classification is a reviewed planning decision and must not be inferred anew by
an upgrader from extensions. An independent inventory gate must reject missing,
duplicate, extra, malformed or unclassified entries and compare every recorded
mode/hash against the baseline, including the symlink blob.
