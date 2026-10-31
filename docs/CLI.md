# Factory CLI reference

Use one public entrypoint, `./factory`, from your project. The script installation
and selected Go installation preserve that path. The Go CLI uses Cobra while
retaining legacy argument behavior; command-specific options are not uniform.
Start with `./factory help`. In particular, do not assume every command supports
`--help`, `--json`, `--version` or a global dry-run flag.

## Which runtime am I using?

As of 2026-10-26 UTC, v0.1.6 is the latest published stable release. The source checkout
on `main` still launches scripts through its tracked `factory` file. Building the
Go command does not activate it in an existing project. A successful explicit
`--go-candidate TAG` installation selects the authenticated packaged Go runtime
behind the same `./factory` entrypoint. The replacement v0.2.0 release and default
source/bootstrap cutover remain pending; see [release readiness](migration/GO_RELEASE_READINESS.md)
and the [upgrade guide](UPGRADING.md).

The table describes the current Go command surface. Entries marked **Go only**
are not commands of the default source shell dispatcher. In a script installation,
use the retained script/Make entrypoints linked below. An older published release
can have fewer features than current `main`.

## Command map

| Command | When to use it | Example and effects |
|---|---|---|
| `help` | Discover the public commands | `./factory help` prints the selected runtime's command list. No model call. |
| `init` | Set up a new project | Go `./factory init ../new-project --pack go,typescript` authenticates its selected installed release or the prepared source default v0.2.0, which must be published before use. Override with an exact stable `--ref TAG` or `FACTORY_REF`; use `--go-candidate TAG` for a published prerelease. `--local --source /path/to/clean-factory` builds committed local source. Prompts for settings and consent, installs the selected runtime and proves gates. May install project dependencies; no model call. See [initialization](migration/INIT.md). |
| `doctor` | Diagnose installed factory health | `./factory doctor` reports armed, inert and stale gates and runs isolated break/fix probes. It does not repair configuration. No model call. |
| `upgrade` | Update the factory installation | Go `./factory upgrade --dry-run --json` previews its declared stable release; current source prepares v0.2.0, which must be published before use. Override with an exact stable `--ref TAG` or `FACTORY_REF`; use `--go-candidate TAG` for an explicit candidate. `upgrade --local --source DIR` builds committed source and needs `--before-source DIR` for Go predecessors. Applying requires a fresh digest and `--quiescent`. See [ordinary Go upgrades](UPGRADING.md#ordinary-go-upgrades). The retained script updater still defaults to main and can overwrite custom framework edits; save them first. |
| `sync` **Go only** | Regenerate harness adapters after canonical role/model changes | `./factory sync all`, or select `opencode`, `claude` or `codex`. Writes derived harness files; inspect their diff. Script equivalent: `make sync-harnesses`. See [orchestration](migration/ORCHESTRATION.md). |
| `prereq` **Go only** | Find missing tools before setup/checks | `./factory prereq` checks tools and the configured product Go minimum. `FACTORY_HARNESS=codex ./factory prereq` selects Codex discovery. Executes tool discovery/version probes. Script equivalent: `make prereq-check`. |
| `check` | Run the pre-push checks | `./factory check` runs the configured gate/build/test stages. These execute project commands and may generate their normal outputs. See [hooks](HOOKS.md). |
| `selftest` | Watch deterministic enforcement reject broken fixtures | The Go command runs fourteen core gate violation/healthy-control pairs in private fixtures, using source assets beside its invocation or the selected installation. No compiler, Python or model call. The tracked source shell launcher and `make selftest` still run the broader historical shell suite until their separate cutover. This is separate from product tests; see [orchestration](migration/ORCHESTRATION.md). |
| `eval` **Go only** | Inspect harness wiring or evaluate agent quality | `./factory eval structural --harness=codex` checks wiring locally. `./factory eval golden --harness mock --runs 1` uses the local mock runner and writes results. Explicit real runners can call paid models. See [evaluation](../eval/README.md). |
| `review` **Go only** | Ask the configured adversarial provider to inspect a saved diff | `./factory review change.diff` sends the diff to the configured provider and can incur charges. It is advisory and does not replace tests or human review. Script path: `scripts/adversarial-review.sh`. |
| `review-batch` **Go only** | Review complete exact-revision diffs in bounded batches | `./factory review-batch --diff change.diff --files files.json --base BASE_SHA --head HEAD_SHA --repository OWNER/REPO --output NEW_DIRECTORY --plan` plans locally and writes reports. Normal execution can call a paid model; GitHub mode also publishes review results. See the boundaries below. |
| `review-lane` | Manage advisory PR review | `./factory review-lane status`, `enable`, `disable`, `pending` or `secret-name`. Enable/disable changes configuration/workflow files; later review jobs consume provider usage. See [review lane](REVIEW_LANE.md). |
| `metrics` | Inspect local repository activity and enforcement | `./factory metrics --json`, `./factory metrics --days 7`, or `./factory metrics --html --no-open`. HTML writes a local page; without `--no-open` it opens your browser. No model calls or upload. See [metrics](METRICS.md). |
| `report` | Read gate/block history and a labelled cost estimate | `./factory report` reports locally; `./factory report --clear` resets the block log. This is distinct from actual invocation accounting in `budget report`. |
| `budget` | Plan, bound or report factory-managed model invocations | `./factory budget plan --harness codex --session work --task issue-1`, `run ... --prompt-file task.md`, or `report --session work --json`. Only explicit `run` invokes a model, with `budget_enabled: true`. See [budgets](BUDGETS.md). |
| `loop` | Run a manual check or a bounded implementation/review/repair loop | `./factory loop plan --harness codex --session work --task issue-1`; actions are `plan`, `run`, `resume` and `status`. Manual checks use no model; bounded mode requires explicit enablement and shares budget accounting. See [loops](LOOPS.md). |
| `migrate-config` | Convert old flat configuration safely | `./factory migrate-config --dry-run` previews conversion of `factory.config` to `factory.yaml`. Remove `--dry-run` only after reviewing; conversion preserves an adjacent backup and refuses conflicting state. It does not activate a Go installation. See [configuration migration](migration/CONFIG_MIGRATION.md). |

## Everyday workflow

From an initialized project:

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

Contributors can build the native command beside the source assets:

```sh
go build -o factory-go ./cmd/factory
./factory-go help
./factory-go sync --help
./factory-go budget --help
./factory-go loop --help
```

Keep the tracked `factory` launcher intact. Ordinary source-built Go init selects
an authenticated release rather than copying the checkout's scaffold. The prepared
default v0.2.0 refuses until published; select an available official prerelease
with `--go-candidate TAG` or build clean committed source with `--local`. Other
source commands can still require assets beside the invoked executable. A detached
development binary does not acquire installed identity.

`cmd/factory-stage` and `cmd/factory-package` are maintainer build/qualification
utilities. They stage verified artifacts and construct release packages; regular
users do not need to learn or install them. Internal bridge protocols and
`--installation` image operands support adapters/maintainer recovery and are not
another daily CLI. Retained direct/sourceable script paths route to the declared
implementation in a selected Go installation. See [orchestration](migration/ORCHESTRATION.md)
and [the installation contract](adr/0098-whole-installation-upgrade-and-rollback.md).
