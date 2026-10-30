# Software Factory Template

**In short**

- A project-agnostic scaffold that wraps AI coding agents in deterministic gates: shell hooks and CI checks that block, instead of prompts that ask.
- Extracted from a factory that built a real product. Nothing here is speculative tooling.
- Every gate is proven by a break/fix fixture — we've watched each one catch the defect it exists for.
- MIT licensed.

## Quick start

```bash
# one shot — fetch, then set up the current repo:
cd your-project
curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- init --pack go

# polyglot? pass several — e.g. a Go backend with a TypeScript frontend:
curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- init --pack go,typescript

# later, from inside the repo — pull framework updates (a reviewable diff):
curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- upgrade

# the installer uses its pinned stable release; --ref takes any branch or tag:
curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- init --ref main
curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- upgrade --ref main
```

The installer only fetches: it clones the template into `~/.software-factory-template` and stops. It runs nothing it downloaded. Prefer to inspect first?

```bash
curl -fsSLO https://softwareaifactory.sh/install.sh && less install.sh && sh install.sh
```

`factory-init.sh` detects your stack, writes `factory.yaml`, installs the hooks, then runs a break/fix self-test so you watch a gate fire before it declares success. Afterward, try pushing straight to `main`, or committing with the message `fix: verified the thing`. Both get blocked. That's the product.

## Upgrade from v0.1.6 to the Go factory

**Release status:** v0.1.6 is the latest published stable release. v0.2.0 is the
planned Go replacement; it is not published yet. Native commands and the explicit
candidate upgrade flow are implemented on `main`.
[v0.2.0-rc.3](https://github.com/anoop2811/software-factory-template/releases/tag/v0.2.0-rc.3)
passed all twenty published scenarios across four native targets, including
init, upgrade, rollback and interruption/recovery. Minimum-OS coverage, the operator pilot
and default cutover remain open; track the
[release checklist](docs/migration/GO_RELEASE_READINESS.md).

An ordinary `upgrade` or `upgrade --ref main` continues to select the script
installation. When v0.2.0 is published with its complete attested assets, use the
explicit Go flow below from your existing project:

```sh
# Preview only; requires the complete published v0.2.0 release.
curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- upgrade --go-candidate v0.2.0 --dry-run --json

# Review the proposal. Stop other factory work for the maintenance window.
# Replace PROPOSAL_DIGEST with the complete proposal_digest from that preview.
curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- upgrade --go-candidate v0.2.0 --confirm PROPOSAL_DIGEST --quiescent

# After successful activation, use the same public command.
./factory doctor
```

For a published prerelease, substitute its exact tag in both commands. The
preview authenticates assets and reports conflicts without changing your project.
Confirmation rechecks the proposal before activation. Unchanged owned obsolete
implementations are replaced or retired; custom files are preserved. Compatible
public script paths remain thin adapters to Go. Recovery copies are inert and
Git-ignored under `.factory/backups/`; qualified later releases prune eligible
older sets while retaining the immediately preceding usable installation.

Read the [upgrade and recovery guide](docs/UPGRADING.md) for prerequisites,
configuration/history preservation, retention exceptions and interrupted upgrades.
See the [CLI command reference](docs/CLI.md) for the commands available in each
runtime. Installing the packaged Go factory requires no Go compiler or Python;
product tools and retained validation gates keep their own requirements.

## Getting started

1. Read [docs/CONCEPTS.md](docs/CONCEPTS.md) (~5 min) — why the gates exist.
2. Use the Quick start initializer in your project to install the hooks and prove they fire.
3. Run `./factory doctor` any time to see which gates are armed, inert, or stale — and watch every armed gate fire on its own break/fix proof.
   - `./factory report` shows what the gates caught, enforced at 0 model tokens, plus one clearly-labeled cost estimate — no vanity "tokens saved" number.
   - `./factory metrics` shows what the factory is doing to your repo: gates armed vs inert, what they blocked, rework, reverts, whether claims carry evidence. Mostly from git history, so it works the day you install. `--json` to export, `--html` for a page. Local only — nothing is transmitted.
4. Read [docs/ADAPTING.md](docs/ADAPTING.md) "Adopting incrementally" to arm gates one at a time.
5. Open [docs/HOOKS.md](docs/HOOKS.md) when a gate blocks you and you want to know why.

## Factory commands

Use one public entrypoint: `./factory`. This table describes the selected Go
installation; commands marked **Go only** are absent from the default script
dispatcher. Use an actual published release tag for `TAG`. Start with
`./factory help`; flags vary by command. The [CLI reference](docs/CLI.md) explains
options, runtime availability and side effects.

| Command | Purpose | Example |
|---|---|---|
| `help` | List the public commands. | `./factory help` |
| `init` | Initialize a project, configure its packs and install/prove hooks. Go callers can explicitly build clean committed local source with `--local --source DIR`; see [initialization](docs/migration/INIT.md). | `./factory init ../new-project --go-candidate TAG --pack go` |
| `doctor` | Diagnose armed, inert and stale gates and adapter integrity. | `./factory doctor` |
| `upgrade` | Authenticate and preview an installation update; apply with fresh confirmation. | `./factory upgrade --go-candidate TAG --dry-run --json` |
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
| `budget` | `plan`, `run` or `report` managed model invocations; only explicit `run` launches a model. | `./factory budget plan --harness codex --session work --task issue-1` |
| `loop` | `plan`, `run`, `resume` or inspect `status` for manual checks or bounded agent work. | `./factory loop plan --harness codex --session work --task issue-1` |
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

The second idea is the Verification Contract: only claim what you've observed. Three claim levels (WROTE / RAN / OBSERVED), every check proven by watching it fail. See [docs/CONCEPTS.md](docs/CONCEPTS.md).

The third idea is that this is already a graph. The vocabulary everyone's chasing — prompt, loop, harness, graph — maps straight onto what's here: roles are the nodes, gates are the verified edges, the reviewer is the skeptic that never wrote the code, and models tier by role. You don't adopt a graph framework; the graph is the floor, and it runs the same on Claude Code, opencode, and Codex because the roles are canonical. The mapping is in [docs/CONCEPTS.md](docs/CONCEPTS.md).

## What's inside

| Piece | Where | What it does |
|---|---|---|
| Agent roles | `.opencode/agent/` | spec-writer, implementer, refactorer, reviewer, wiki-maintainer. The role that writes tests never writes implementation — enforced by a hook, not by trust. |
| Hooks | `scripts/hooks/` | Deterministic gates on commits, pushes, and CI. Each one in [docs/HOOKS.md](docs/HOOKS.md). |
| Harness adapters | `factory sync` in Go; `scripts/sync-*.sh` in the script installation | opencode is canonical; the Claude Code and Codex configs are generated from it, and CI fails on drift. |
| Language packs | `packs/` | The core never mentions a language. Packs carry the stack opinions. |
| Agent evals | `eval/` | Score agents on real tasks — a red spec plus an oracle, pass rate over N runs. Baselines record what they were measured against, so a changed oracle reports *stale* instead of a false "no regression". |
| Workflow recipes | `workflows/` | The graph in plain text: nodes are roles, edges are code, findings pass a verifier. `workflow-lint` enforces the shape; one recipe runs natively on all three harnesses. |
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
The [command reference](docs/CLI.md) distinguishes this source surface from the
current script default; [release readiness](docs/migration/GO_RELEASE_READINESS.md)
tracks the remaining qualification and cutover gates.

## Cost & models

`./factory budget plan --harness codex --session work --task issue-1` previews
limits without calling a model. The opt-in `run` command bounds factory-managed
invocations and records metadata for Codex, Claude Code, and OpenCode. Reports
distinguish reported tokens, client cost estimates, and unknown usage; they do
not claim a strict dollar cap. See [budget controls](docs/BUDGETS.md).

For manual checks or explicitly bounded implement/check/review/repair, see
[Loop engineering](docs/LOOPS.md). `factory loop plan` makes no model calls.

Each role runs on a cost-appropriate model. The reviewer and spec-writer get a frontier model; the low-stakes roles — refactorer, wiki-maintainer, background tasks — get a cheaper one. Because opencode, Claude, and Codex have different native model namespaces, each carries its own per-tier defaults (verified current, overridable in `factory.yaml`):

| Tier | opencode (OpenRouter) | Codex | Claude |
|---|---|---|---|
| frontier | `openrouter/z-ai/glm-5.2` | `gpt-5.6-sol` | `claude-opus-4-8` |
| default | `openrouter/z-ai/glm-5.2` | `gpt-5.6-terra` | `claude-sonnet-4-6` |
| economy | `openrouter/qwen/qwen3-coder` | `gpt-5.6-luna` | `claude-haiku-4-5` |

It's a routing change only — no gate is relaxed, so the same hooks check the output whichever model produced it. That's what lets a cheaper model run safely on the low-stakes roles. The default (`standard`) profile runs two tiers — frontier and default; the opt-in `economy` profile adds the cheaper third. To reconfigure later, edit `factory.yaml` (change a model, or flip `cost_profile`) and run `make sync-harnesses` — one edit re-routes all three harnesses. See [docs/COST_AND_TOKENS.md](docs/COST_AND_TOKENS.md).

## Documentation

- [docs/CLI.md](docs/CLI.md) — commands, examples, runtime availability and model-cost choices
- [docs/UPGRADING.md](docs/UPGRADING.md) — v0.1.6 to the Go factory, backups and recovery
- [docs/migration/GO_RELEASE_READINESS.md](docs/migration/GO_RELEASE_READINESS.md) — remaining Go replacement release gates
- [docs/CONCEPTS.md](docs/CONCEPTS.md) — why the rules exist, including the full Verification Contract
- [docs/ADAPTING.md](docs/ADAPTING.md) — adopting the factory in an existing project
- [docs/COST_AND_TOKENS.md](docs/COST_AND_TOKENS.md) — the cost profile and per-harness model tiers
- [docs/MODELS.md](docs/MODELS.md) — choosing a model provider; blank means inherit, so no provider is assumed
- [docs/REVIEW_LANE.md](docs/REVIEW_LANE.md) — the opt-in adversarial PR review: what it costs, and the privilege boundary
- [docs/METRICS.md](docs/METRICS.md) — what the factory is doing to your repo; local only, no exporters, no server
- [docs/LOOPS.md](docs/LOOPS.md) — manual checks, bounded repair and checkpoint handoffs
- [docs/FEATURE_ROADMAP.md](docs/FEATURE_ROADMAP.md) — stable feature priorities and completion tracking
- [docs/WORKFLOWS.md](docs/WORKFLOWS.md) — workflow recipes (graph engineering) and the workflow-lint gate
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
