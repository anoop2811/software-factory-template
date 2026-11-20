# Factory CLI reference

The [v0.2.0 stable release](https://github.com/anoop2811/software-factory-template/releases/tag/v0.2.0) provides one project command, `./factory`. Use it to bootstrap language packs and CI, configure Codex, Claude Code and OpenCode, run checks, plan agent work and upgrade the installation. Start with `./factory help`. Options such as `--json` and `--dry-run` belong to particular commands.

## Which runtime am I using?

A v0.2.0 installation runs a packaged Go binary. Look for `"version":"v0.2.0"` in `.factory/installation.current` at the project root. A v0.1.6 installation still runs Bash until you preview and confirm a migration. Follow the [upgrade guide](UPGRADING.md) from that project. Contributors running the source checkout use the same `./factory` entrypoint with a local Go toolchain and module cache.

With a new project, start with the [initializer and pack choices](ADAPTING.md). The table below describes the released v0.2.0 CLI. **Go only** identifies commands absent from the v0.1.6 dispatcher.

## Command map

| Command | When to use it | Example and effects |
|---|---|---|
| `help` | Discover the public commands | `./factory help` prints the selected runtime's command list. No model call. |
| `init` | Set up a new project | Go `./factory init ../new-project --pack go,typescript` uses the selected installed release, or v0.2.0 from this source revision. To choose another release, pass `--ref TAG` for a stable tag or `--go-candidate TAG` for a prerelease. `--local --source /path/to/clean-factory` builds committed local source. Prompts for settings and consent, installs the selected runtime and proves gates. May install project dependencies; no model call. See [initialization](migration/INIT.md). |
| `doctor` | Diagnose installed factory health | `./factory doctor` reports armed, inert and stale gates and runs isolated break/fix probes. It does not repair configuration. No model call. |
| `upgrade` | Update the factory installation | Go `./factory upgrade --dry-run --json` previews the stable release declared by the running controller (v0.2.0 in this release). Override with an exact stable `--ref TAG` or `FACTORY_REF`; use `--go-candidate TAG` for an explicit candidate. `upgrade --local --source DIR` builds committed source and needs `--before-source DIR` for Go predecessors. Apply with `--confirm PROPOSAL_DIGEST --quiescent` after reviewing a fresh preview. See [ordinary Go upgrades](UPGRADING.md#ordinary-go-upgrades). The direct source upgrade adapter uses this same Go controller. |
| `ci prepare` **Go only** | Prepare a disposable checkout's missing private runtime | Invoke a separately trusted native binary: `/path/to/trusted/factory ci prepare --go-candidate TAG --origin-target darwin/arm64 --ephemeral`, or use the [authenticated installer entrypoint](migration/CI_PREPARATION.md#bootstrap-without-an-existing-native-executable). Authenticates exact origin/host release images and reprojects clean tracked factory assets for this job. No model call. This is not an upgrade or repair command. |
| `ci review-client` **Go only** | Acquire a standalone review executable in disposable scratch | `/path/to/trusted/factory ci review-client --go-candidate TAG --expected-revision COMMIT --output /absolute/new/client-directory`. Requires an exact release and matching full source commit; writes one private executable and emits a JSON identity receipt. The trusted installer exposes the same operation. No project mutation, compiler, Python or model call. See [bootstrap scope and prerequisites](migration/CI_PREPARATION.md#standalone-review-client). |
| `ci workflow` **Go only** | Render a workflow for an installed factory | `./factory ci workflow --go-candidate TAG --origin-target darwin/arm64 --installer-revision COMMIT` prints GitHub Actions YAML. All selectors are explicit; `COMMIT` is a reviewed lowercase forty-character installer commit SHA. No project inspection, acquisition, file publication or model call. Review application setup and existing workflows before installation; see [workflow generation](migration/CI_PREPARATION.md#render-a-workflow-for-review). |
| `sync` **Go only** | Regenerate harness adapters after canonical role/model changes | `./factory sync all`, or select `opencode`, `claude` or `codex`. Writes derived harness files; inspect their diff. Script equivalent: `make sync-harnesses`. See [orchestration](migration/ORCHESTRATION.md). |
| `prereq` **Go only** | Find missing tools before setup/checks | `./factory prereq` checks tools and the configured product Go minimum. `FACTORY_HARNESS=codex ./factory prereq` selects Codex discovery. Executes tool discovery/version probes. Script equivalent: `make prereq-check`. |
| `check` | Run the pre-push checks | `./factory check` runs the configured gate/build/test stages. These execute project commands and may generate their normal outputs. Local Git activation is opt-in; see [full pre-push setup](HOOKS.md#full-pre-push-checks). |
| `selftest` | Watch deterministic enforcement reject broken fixtures | `./factory selftest`, `scripts/selftest/run.sh` and `make selftest` run fourteen core gate violation/healthy-control pairs in private fixtures, using source assets beside the invocation or the selected installation. The native proof engine uses no compiler, Python or model; source invocation first builds the CLI. Factory contributors run `scripts/selftest/development.sh` separately for the broader current and historical regression suite, also selected by this repository's configured check. See [orchestration](migration/ORCHESTRATION.md). |
| `eval` **Go only** | Inspect harness wiring or evaluate agent quality | `./factory eval structural --harness=codex` checks wiring locally. `./factory eval golden --harness mock --runs 1` uses the local mock runner and writes results. Explicit real runners can call paid models. See [evaluation](../eval/README.md). |
| `review` **Go only** | Ask the configured adversarial provider to inspect a saved diff | `./factory review change.diff` sends the diff to the configured provider and can incur charges. It is advisory and does not replace tests or human review. Script path: `scripts/adversarial-review.sh`. |
| `review-batch` **Go only** | Review complete exact-revision diffs in bounded batches | `./factory review-batch --diff change.diff --files files.json --base BASE_SHA --head HEAD_SHA --repository OWNER/REPO --output NEW_DIRECTORY --plan` plans locally and writes reports. Normal execution can call a paid model; GitHub mode also publishes review results. See the boundaries below. |
| `review-lane` | Manage advisory PR review | `./factory review-lane status`, `enable`, `disable`, `pending` or `secret-name`. Enable/disable changes configuration/workflow files; later review jobs consume provider usage. See [review lane](REVIEW_LANE.md). |
| `metrics` | Inspect local repository activity and enforcement | `./factory metrics --json`, `./factory metrics --days 7`, or `./factory metrics --html --no-open`. HTML writes a local page; without `--no-open` it opens your browser. No model calls or upload. See [metrics](METRICS.md). |
| `report` | Read gate/block history and a labelled cost estimate | `./factory report` reports locally; `./factory report --clear` resets the block log. This is distinct from actual invocation accounting in `budget report`. |
| `budget` **Go only** | Plan, bound or report factory-managed model invocations | `./factory budget plan --harness codex --session work --task issue-1`, `run ... --prompt-file task.md`, or `report --session work --json`. Only explicit `run` invokes a model, with `budget_enabled: true`. See [budgets](BUDGETS.md). |
| `loop` **Go only** | Run a manual check or a bounded implementation/review/repair loop | `./factory loop plan --harness codex --session work --task issue-1`; actions are `plan`, `run`, `resume` and `status`. Manual mode runs the configured check once; bounded mode requires explicit enablement and shares budget accounting. See [loops](LOOPS.md). |
| `migrate-config` | Convert old flat configuration safely | `./factory migrate-config --dry-run` previews conversion of `factory.config` to `factory.yaml`. Remove `--dry-run` only after reviewing; conversion preserves an adjacent backup and refuses conflicting state. It does not activate a Go installation. See [configuration migration](migration/CONFIG_MIGRATION.md). |

The three `ci` subcommands use the flag name `--go-candidate` for both stable and prerelease tags. Use `v0.2.0` for `TAG` to select this release. Generated workflows pin the installer revision and record the installation’s actual origin target.

**Go-pack prerequisite check in v0.2.0.** The initializer can write a two-part `go_min_version` such as `1.26`. The released `prereq` command requires a three-part minimum and reports “cannot be compared” even for a newer installed Go. Set `go_min_version` in `factory.yaml` to the full minimum patch your project supports, then run `./factory prereq` again.

## Everyday workflow

From an initialized project with a Git commit and a configured check command:

```sh
./factory help
./factory doctor
./factory budget plan --harness codex --session maintenance --task issue-1
./factory budget report --session maintenance --json
./factory loop plan --harness codex --session maintenance --task issue-1
./factory metrics --json
```

Plans/reports do not launch models. A budget plan can return exit 2 with a disabled
or exhausted-budget explanation; that is not an instruction to enable spending.
Use `--harness claude` or `--harness opencode` for those adapters. `doctor` executes
gate probes, and `check`/manual loops execute your configured project commands;
review those commands as you would any build/test tooling.

Authenticated v0.2.0 initialization also writes
`.github/workflows/factory.yml` with installed-runtime preparation and the selected
language-pack setup. Its ordinary PR evaluation is mock-only. Existing application
workflows are preserved; an occupied `factory.yml` refuses initialization before
publication. Local-source init cannot generate a downloadable release workflow.
See [initialized CI](migration/CI_PREPARATION.md#ci-created-by-current-initialization)
and [the opt-in review lane](REVIEW_LANE.md#runner-runtime) for authenticated
runtime preparation and workflow migration. The [stable release record](migration/GO_RELEASE_READINESS.md#stable-v020) lists the qualified targets and migration exercises.

## Explicit model execution

`budget run`, bounded `loop run`, live evaluation, `review` and non-plan
`review-batch` can consume paid usage. Budget/loop limits cover factory-managed
invocations, not unrelated interactive work or every model turn inside a harness.
Review batching has its own request/token/input/time allowances; it does not debit
the budget/loop ledger. Reported client costs are estimates, not billing ceilings.

For a bounded loop, set both `budget_enabled: true` and `loop_enabled: true`,
review limits in [BUDGETS.md](BUDGETS.md) and [LOOPS.md](LOOPS.md), then deliberately run:

```sh
./factory loop run --mode bounded --harness codex --session feature-work --task issue-1 --prompt-file task.md
```

A separate reviewer consumes attempts too. Read `loop status` and its next action
after an interruption; bounded `resume` does not restart model calls automatically.

## Batched review and CI

Local/offline mode takes a complete Git diff, GitHub-format files manifest, exact
base/head commit IDs, repository and new output directory under real, nonsymlinked
parents. Supply `--diff`,
`--files`, `--base`, `--head`, `--repository`, `--output`, plus optional `--plan`
and `--cache`. Only this mode supports `--plan`: it reads your supplied files and
writes reports without contacting a provider. Removing `--plan` enables configured
OpenRouter review and requires the review API key.

`./factory review-batch github --pr NUMBER --repository OWNER/REPO --output NEW_DIRECTORY`
is the CI integration. It requires `GITHUB_TOKEN` with the necessary repository,
comment and check permissions, fetches same-repository PR data, calls the configured
review engine and publishes its coverage/review result to GitHub. It does not
support `--plan`; do not use it as an offline preview. Explicit `--expected-base`
and `--expected-head` selectors pin review to the intended full commit SHAs.
See [the batching contract](adr/0100-batched-review.md).

Completion reports measure file/fragment coverage; they are not approval. A failed
or incomplete review remains incomplete. The lane executes its trusted base
client; PR content is untrusted review data, never executable setup.

## Maintainer tools and compatibility adapters

Contributors can use `./factory` directly with the provisioned Go toolchain and
module cache. It builds offline, preserves caller input and publishes one ignored
developer output. Provision the locked modules with `go mod download` first. A failed source build preserves the previous developer output and exits before invoking it.
Concurrent commands retain separate private executables until each returns;
normal completion removes only its own private file. They can also build the
native command explicitly:

```sh
go build -o factory-go ./cmd/factory
./factory-go help
./factory-go sync --help
./factory-go budget --help
./factory-go loop --help
```

Keep the tracked `factory` launcher intact. Ordinary source-built Go init selects
an authenticated release rather than copying the checkout's scaffold. The default is v0.2.0. Use `--ref TAG` to pin a stable release,
`--go-candidate TAG` to select a prerelease, or `--local` to build committed source. Other
source commands can still require assets beside the invoked executable. A detached
development binary does not acquire installed identity.

`cmd/factory-stage` and `cmd/factory-package` are maintainer build/qualification
utilities. They stage verified artifacts and construct release packages; regular
users do not need to learn or install them. Internal bridge protocols and
`--installation` image operands support adapters/maintainer recovery and are not
another daily CLI. Retained direct/sourceable script paths route to the declared
implementation in a selected Go installation. See [orchestration](migration/ORCHESTRATION.md)
and [the installation contract](adr/0098-whole-installation-upgrade-and-rollback.md).
