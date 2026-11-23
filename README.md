# Software Factory Template

Software AI Factory bootstraps a new or existing repository for development with Codex, Claude Code and OpenCode. Give the next developer a working set of language-pack files, shared agent roles, project checks and GitHub Actions without assembling a separate setup for each tool.

[v0.2.0](https://github.com/anoop2811/software-factory-template/releases/tag/v0.2.0) ships one project-local Go CLI. Start with setup and local checks. Turn on budgeted agent runs, bounded repair loops or model-assisted PR review when you need them. [Explore the commands](docs/CLI.md).

## Quick start

Run `gh auth status` before using the authenticated Go installer for `init` or `upgrade`. If this is your first time using the GitHub CLI, run `gh auth login`.

```bash
# one shot — fetch, then set up the current repo:
cd your-project
curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- init --ref v0.2.0 --pack go

# polyglot? pass several — e.g. a Go backend with a TypeScript frontend:
curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- init --ref v0.2.0 --pack go,typescript

# From an existing v0.1.6 project, preview the upgrade before confirming it:
curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- upgrade --ref v0.2.0 --dry-run --json

# From an installed v0.2.0 project, preview this version with its own Go CLI:
./factory upgrade --ref v0.2.0 --dry-run --json
```

The pack checks start at the project root. For a monorepo with nested modules or workspaces, adapt the check command and workflow paths to your directory layout.

With no subcommand the installer only fetches the pinned source. With `init` it authenticates the published Go CLI using GitHub CLI (`gh`), prompts for project settings, installs the selected assets and runs a break/fix self-test. To inspect the installer first, download it with `curl -fsSLO https://softwareaifactory.sh/install.sh`, read it, then run `sh install.sh init --ref v0.2.0 --pack go`. Local Git hooks need explicit activation: follow the [full pre-push check setup](docs/HOOKS.md#full-pre-push-checks), preserving any existing hooks. The tracked `.githooks/pre-push` only blocks direct updates to `main`. Commit-message lint runs in the configured CI checks or through `make lint-commits`; init does not install a local `commit-msg` hook.

## Upgrade from v0.1.6 to the Go factory

[v0.2.0](https://github.com/anoop2811/software-factory-template/releases/tag/v0.2.0) ships the Go factory for Ubuntu 24.04+ and macOS 15+ on amd64 and arm64. The installer uses the default built into its own revision. Use the exact tag for both steps so the preview and confirmation select the same release. On the original v0.1.6 installation, a bare local `./factory upgrade` defaults to `main` unless overridden. Use the authenticated installer below for this migration. Keep `--ref v0.2.0` on the preview so an older shell installer cannot select the legacy updater, which omits the preview flags.

```sh
# Preview changes without modifying the project.
curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- upgrade --ref v0.2.0 --dry-run --json

# Review the proposal. Stop other factory work for the maintenance window.
# Replace PROPOSAL_DIGEST with the complete proposal_digest from that preview.
curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- upgrade --ref v0.2.0 --confirm PROPOSAL_DIGEST --quiescent

# After successful activation, use the same public command.
./factory doctor
```

For a published prerelease, replace `--ref v0.2.0` with `--go-candidate TAG` in
both commands, using its exact published tag. The
preview authenticates assets and reports conflicts without changing your project.
Confirmation rechecks the proposal before activation. Unchanged owned obsolete
implementations are replaced or retired; custom files are preserved. Compatible
public script paths remain thin adapters to Go. Recovery copies are inert and
Git-ignored under `.factory/backups/`; qualified later releases prune eligible
older sets while retaining the immediately preceding usable installation.

Read the [upgrade and recovery guide](docs/UPGRADING.md) for prerequisites,
configuration/history preservation, retention exceptions and interrupted upgrades.
For an earlier experimental Go installation, use the freshly downloaded installer
for both preview and confirmation: its old reader may reject metadata from a newer
compiler before handing off. The installer authenticates the new owner first;
see [earlier Go candidates](docs/UPGRADING.md#earlier-experimental-go-installations).
See the [CLI command reference](docs/CLI.md) for the commands available in each
runtime. Installed projects use their packaged Go CLI. The tracked source
launcher builds the CLI with the local Go toolchain and module cache.
A bare installer invocation only fetches source and prints the authenticated
initialization command to use next.
Installing the packaged Go factory requires no Go compiler or Python;
product tools and retained validation gates keep their own requirements.

A fresh v0.2.0 project can contain a copy of the README written before publication. For the shipped command behavior, use the [maintained CLI reference](https://github.com/anoop2811/software-factory-template/blob/main/docs/CLI.md) and [upgrade guide](https://github.com/anoop2811/software-factory-template/blob/main/docs/UPGRADING.md).

## Getting started

1. Read [docs/CONCEPTS.md](docs/CONCEPTS.md) (~5 min) — why the gates exist.
2. Use the Quick start initializer in your project to install the hooks and prove they fire.
3. Run `./factory doctor` any time to see which gates are armed, inert, or stale and exercise the 14 core gate proofs against isolated fixtures.
   - `./factory report` shows what the gates caught, enforced at 0 model tokens, plus one clearly-labeled cost estimate — no vanity "tokens saved" number.
   - `./factory metrics` shows what the factory is doing to your repo: gates armed vs inert, what they blocked, rework, reverts, whether claims carry evidence. Mostly from git history, so it works the day you install. `--json` to export, `--html` for a page. Local only — nothing is transmitted.
4. Read [docs/ADAPTING.md](docs/ADAPTING.md) "Adopting incrementally" to arm gates one at a time.
5. Open [docs/HOOKS.md](docs/HOOKS.md) when a gate blocks you and you want to know why.

## Factory commands

The table covers the released v0.2.0 CLI. Start with `./factory help`. Commands marked **Go only** were absent from the v0.1.6 dispatcher. Use an exact published release for `TAG`; the CI helpers accept `--go-candidate v0.2.0` for the stable release, too. The [CLI reference](docs/CLI.md) explains flags, prerequisites and side effects.

| Command | Purpose | Example |
|---|---|---|
| `help` | List the public commands. | `./factory help` |
| `init` | Initialize a project with its selected authenticated Go release. Current initialization creates ordinary installed CI for the selected language packs and preserves application workflows. Override with stable `--ref TAG`, prerelease `--go-candidate TAG` or clean committed `--local --source DIR`; see [initialization](docs/migration/INIT.md). | `./factory init ../new-project --pack go` |
| `doctor` | Diagnose armed, inert and stale gates and adapter integrity. | `./factory doctor` |
| `upgrade` | Authenticate a stable Go update and preview changes; override with `--ref TAG`, then apply with fresh confirmation. See [ordinary upgrades](docs/UPGRADING.md#ordinary-go-upgrades). | `./factory upgrade --dry-run --json` |
| `ci prepare` **Go only** | Restore the exact release's private runtime in a fresh disposable checkout using a separately trusted native executable. See [CI preparation and its integration boundary](docs/migration/CI_PREPARATION.md). | `/path/to/trusted/factory ci prepare --go-candidate TAG --origin-target darwin/arm64 --ephemeral` |
| `ci review-client` **Go only** | Acquire an authenticated standalone review executable in new disposable scratch; no project changes or model call. See [review-client bootstrap](docs/migration/CI_PREPARATION.md#standalone-review-client). | `/path/to/trusted/factory ci review-client --go-candidate TAG --expected-revision COMMIT --output /absolute/new/client-directory` |
| `ci workflow` **Go only** | Render an installed-factory GitHub workflow to stdout for review; preserves existing workflow files. See [workflow generation](docs/migration/CI_PREPARATION.md#render-a-workflow-for-review). | `./factory ci workflow --go-candidate TAG --origin-target darwin/arm64 --installer-revision COMMIT` |
| `sync` **Go only** | Regenerate adapters for `all`, `codex`, `claude` or `opencode`; inspect the resulting diff. | `./factory sync all` |
| `prereq` **Go only** | Check required tools and the configured product Go minimum. | `./factory prereq` |
| `check` | Run factory gates and your configured project checks. | `./factory check` |
| `selftest` | Run isolated break/fix proofs of factory enforcement. | `./factory selftest` |
| `eval` **Go only** | Check harness wiring or run structural/golden evaluations. Real runners can consume model usage. | `./factory eval structural --harness=codex` |
| `review` **Go only** | Send a saved diff to the configured advisory reviewer; consumes provider usage. | `./factory review change.diff` |
| `review-batch` **Go only** | Plan or execute complete reviews in bounded batches; local `--plan` makes no provider calls. | [Offline plan and GitHub examples](docs/CLI.md#batched-review-and-ci) |
| `review-lane` | Inspect/manage advisory PR review: `status`, `enable`, `disable`, `pending`, `secret-name`. | `./factory review-lane status` |
| `metrics` | Read local activity/enforcement metrics; JSON or local HTML output is available. | `./factory metrics --json` |
| `report` | Read gate/block history and a labelled estimate; `--clear` resets the block log. | `./factory report` |
| `budget` **Go only** | `plan`, `run` or `report` managed model invocations; only explicit `run` launches a model. | `./factory budget plan --harness codex --session work --task issue-1` |
| `loop` **Go only** | `plan`, `run`, `resume` or inspect `status` for manual checks or bounded agent work. | `./factory loop plan --harness codex --session work --task issue-1` |
| `migrate-config` | Preview conversion of legacy flat configuration to YAML; applying preserves a backup. | `./factory migrate-config --dry-run` |

Budgeted model runs and bounded loops require explicit enablement. Real evaluation
runners, `review`, executed `review-batch` and enabled PR review can consume model
usage; planning and local reports do not launch models. Read [budget limits](docs/BUDGETS.md)
and [loop controls](docs/LOOPS.md) before enabling agent execution.

For rollback, interrupted recovery and `upgrade --keep-backup MIGRATION_ID` /
`upgrade --prune-backup MIGRATION_ID`, follow the [upgrade and recovery guide](docs/UPGRADING.md).
The packaging/staging utilities are maintainer tools; regular users keep the one
`factory` CLI. See [maintainer tools](docs/CLI.md#maintainer-tools-and-compatibility-adapters).

## The idea

A prompt that says "never edit test files" is a request an agent may or may not honor. A shell hook that exits 2 on the edit is a fact. Put the control in a hook, not in the prompt — that principle is due to Birgitte Böckeler (Thoughtworks), and this template applies it to commit discipline, push gates, role separation, and citation integrity.

The second idea is the Verification Contract: only claim what you've observed. Its three claim levels are WROTE, RAN and OBSERVED. When adding a gate, exercise a deliberate failure and a healthy control. The v0.2.0 installed self-test covers fourteen such pairs. See [docs/CONCEPTS.md](docs/CONCEPTS.md).

Workflow recipes describe which role handles each step and where findings get checked. The repository ships review and evaluation recipes. Each harness supplies its own orchestration. See [workflows](docs/WORKFLOWS.md).

## Bounded delivery

Keep each PR focused on one complete, observable function. Include its independent
tests, compatibility behavior, required cleanup and documentation in that PR.
This gives human and AI reviewers one coherent change to follow from requirement
to evidence. Independent functions can proceed on separate branches; dependent
changes merge in order after their own reviews and checks pass.

Use this planning practice with Codex, Claude Code or OpenCode. Review batching
and request budgets provide separate computational limits; the factory does not
enforce a universal PR line limit. See the [bounded delivery guide](docs/BOUNDED_DELIVERY.md)
for scope examples and the merge workflow.

## What's inside

| Piece | Where | What it does |
|---|---|---|
| Agent roles | `.opencode/agent/` | spec-writer, implementer, refactorer, reviewer, wiki-maintainer. With an explicit implementer role and configured test patterns, the test-edit hook blocks edits to matching tests. |
| Hooks | `scripts/hooks/` | Deterministic gates on commits, pushes, and CI. Each one in [docs/HOOKS.md](docs/HOOKS.md). |
| Harness adapters | `factory sync` in Go; `scripts/sync-*.sh` in the script installation | opencode is canonical; the Claude Code and Codex configs are generated from it, and CI fails on drift. |
| Language packs | `packs/` | Select stack-specific checks and tool configuration, one pack or several. |
| Agent evals | `eval/` | Score agents on real tasks — a red spec plus an oracle, pass rate over N runs. Baselines record what they were measured against, so a changed oracle reports *stale* instead of a false "no regression". |
| Workflow recipes | `workflows/` | The graph in plain text: nodes are roles, edges are code, findings pass a verifier. `workflow-lint` enforces the shape; agents interpret the recipe with their own orchestration tools. |
| CLI | `./factory` | One developer entrypoint for setup, health, upgrades, budgets and loops. [Command reference](docs/CLI.md). |
| Metrics | `factory metrics` | What the factory is doing to your repo, computed locally and never sent anywhere. Shows the unflattering numbers — inert gates, repeat blocks, rework, stale baselines — as prominently as the wins. [docs/METRICS.md](docs/METRICS.md). |

## Pack maturity

| Pack | Stack | Maturity |
|---|---|---|
| Go | Ginkgo v2 + Gomega, golangci-lint, gosec, govulncheck, gremlins | battle-tested |
| TypeScript / React / Vue | Vitest, Biome (React/Vue-aware format + lint), tsc, Stryker, OSV-Scanner | experimental |
| Java / Spring Boot | JUnit 5 + AssertJ + Testcontainers, Spotless, Error Prone, SpotBugs + find-sec-bugs, OSV-Scanner, PIT; Gradle or Maven | beta |

Labels are evidence, not roadmap: battle-tested means a real project shipped under the pack, beta means at least one real repo adopted it, experimental means no proven adopter yet (the label tracks adoption, not completeness).

Frameworks ride on the language pack, not their own: React and Vue use the TypeScript pack (Biome auto-applies its `react`/`vue` rules), Spring Boot uses the Java pack (JUnit 5 + Testcontainers). `factory-init` detects them and points you at the right pack. Install several at once for polyglot repos — `--pack go,typescript`.

The [Go runtime conversion spec](specs/001-go-runtime-conversion.md) governs a
staged migration with backward compatibility, recovery and safe cleanup. Native Go implementations now cover setup, health, reporting, metrics, review-lane
configuration, configuration migration, budgets, loops, harness synchronization,
check orchestration, evaluation and review. Authenticated whole-installation
upgrade, rollback, interrupted recovery and backup retention are implemented.
The [command reference](docs/CLI.md) covers the shipped Go CLI. The [stable release record](docs/migration/GO_RELEASE_READINESS.md#stable-v020) links the four native qualification reports and describes the v0.1.6 adopter exercise.

## Cost & models

`./factory budget plan --harness codex --session work --task issue-1` previews
limits without calling a model. The opt-in `run` command bounds factory-managed
invocations and records metadata for Codex, Claude Code, and OpenCode. Reports
distinguish reported tokens, client cost estimates, and unknown usage; they do
not claim a strict dollar cap. See [budget controls](docs/BUDGETS.md).

For manual checks or explicitly bounded implement/check/review/repair, see
[Loop engineering](docs/LOOPS.md). `factory loop plan` makes no model calls.

Keep each harness’s own model configuration by choosing `inherit` during setup, or configure model tiers in `factory.yaml`. The standard profile uses frontier and default tiers. Economy adds a third tier for selected roles. After changing the settings, run `./factory sync all` and inspect the generated diff. See [the model guide](docs/MODELS.md) and [cost profiles](docs/COST_AND_TOKENS.md).

## Documentation

- [docs/CLI.md](docs/CLI.md) — commands, examples, runtime availability and model-cost choices
- [docs/UPGRADING.md](docs/UPGRADING.md) — v0.1.6 to the Go factory, backups and recovery
- [docs/migration/GO_RELEASE_READINESS.md](docs/migration/GO_RELEASE_READINESS.md) — v0.2.0 qualification reports and migration evidence
- [docs/CONCEPTS.md](docs/CONCEPTS.md) — why the rules exist, including the full Verification Contract
- [docs/ADAPTING.md](docs/ADAPTING.md) — adopting the factory in an existing project
- [docs/COST_AND_TOKENS.md](docs/COST_AND_TOKENS.md) — the cost profile and per-harness model tiers
- [docs/MODELS.md](docs/MODELS.md) — choosing a model provider; blank means inherit, so no provider is assumed
- [docs/REVIEW_LANE.md](docs/REVIEW_LANE.md) — the opt-in adversarial PR review: what it costs, and the privilege boundary
- [docs/METRICS.md](docs/METRICS.md) — what the factory is doing to your repo; local only, no exporters, no server
- [docs/LOOPS.md](docs/LOOPS.md) — manual checks, bounded repair and checkpoint handoffs
- [docs/FEATURE_ROADMAP.md](docs/FEATURE_ROADMAP.md) — stable feature priorities and completion tracking
- [docs/WORKFLOWS.md](docs/WORKFLOWS.md) — workflow recipes (graph engineering) and the workflow-lint gate
- [docs/BOUNDED_DELIVERY.md](docs/BOUNDED_DELIVERY.md) — complete-function PRs, parallel branches and ordered merges
- [docs/HOOKS.md](docs/HOOKS.md) — every hook: when it fires, exit codes, what a failure looks like
- [docs/PATTERNS.md](docs/PATTERNS.md) — failure patterns we hit in practice, and the fixes
- [docs/FACTORY_RULES.md](docs/FACTORY_RULES.md) — the operational rulebook agents read
- [docs/GLOSSARY.md](docs/GLOSSARY.md) — terms defined
- [specs/TEMPLATE.md](specs/TEMPLATE.md) — a spec template to copy
- [eval/README.md](eval/README.md) — how the template evaluates agent quality

## Contributing

Contributions are welcome. The template runs its own factory checks and
acceptance fixtures. [Self-hosting](docs/SELF_HOSTING.md) explains local hook
activation, CI review and the boundaries between source templates and adopters.

- [CONTRIBUTING.md](CONTRIBUTING.md) — the workflow, the gates, and the break/fix fixture rule
- [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) — the standards we hold each other to
- [SECURITY.md](SECURITY.md) — reporting vulnerabilities privately

New to the codebase? Start with [docs/GLOSSARY.md](docs/GLOSSARY.md) and
[docs/CONCEPTS.md](docs/CONCEPTS.md). Bug reports and feature requests have issue
templates; PRs have a checklist tied to the gates above.

## License

MIT. See [LICENSE](LICENSE).
