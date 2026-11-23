# Adapting the factory to your project

## Set up a project

Use v0.2.0 to add a shared development workflow to a new or existing Git
project. From its root, check `gh auth status`, then run the public installer
with your chosen language pack:

```sh
curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- init --ref v0.2.0 --pack go
```

The installer authenticates the packaged Go release. The initializer collects
project settings and asks for confirmation before writing the scaffold. It
supplies the project-local `./factory`, canonical roles, adapters for OpenCode,
Claude Code and Codex, shared hooks and the selected pack's checks. Authenticated
setup generates one `.github/workflows/factory.yml` for the selected packs.
An existing workflow at that path blocks initialization. Other application
workflows are preserved. [Initialization details](migration/INIT.md).

Choose `--pack go,typescript` for both packs, or `--pack none` to start with
the core. The stack hints come from manifests at the project root. Review
`check_command` for your repository layout and install any tools your checks
need. The factory itself makes no model call during initialization.

For a Go pack, check the generated `go_min_version` in `factory.yaml`. v0.2.0 can write a two-part version, but `./factory prereq` expects a full minimum patch version. Use your project’s approved minimum before running that check.

## factory doctor

Run `./factory doctor` after setup or when a project rule seems inactive. It
reports gate configuration, missing core hooks, adapter drift, protected-path
coverage in CODEOWNERS and the selected Git hook path. Warnings can accompany
a healthy exit. Its self-test exercises fourteen shipped gates with a violation
and a healthy control in isolated fixtures. Use `./factory check` to exercise
your configured project checks. See the [CLI reference](CLI.md) and
[local Git hook setup](HOOKS.md#full-pre-push-checks).

## factory.yaml reference

The Go CLI reads this file at runtime. Shell hooks use its `scripts/lib/config.sh` adapter. Format: flat `key: value` pairs, no nesting; lists are space-separated on one line; values may be double-quoted; a trailing `# comment` is stripped, and a `#` inside quotes is part of the value. Anything more expressive belongs in a hook, not in configuration.

| Key | Meaning | Example |
|---|---|---|
| `project_name` | Human-readable name, used in agent role prompts and messages. | `myproject` |
| `decision_log` | Path to the decision log that `decision-log-gate.sh` checks references against. | `docs/DECISION_LOG.md` |
| `docs_root` | Root of the spec/docs tree that citations resolve against. | `docs` |
| `citation_prefix` | Prefix for `PREFIX_*.md:NN` citations checked by `citation-lint.sh`. Empty disables the check. | `MYPROJECT_` |
| `protected_paths` | Space-separated path prefixes treated as governance-sensitive: commits touching them must reference a Decision. | `"internal/billing scripts/hooks"` |
| `test_file_patterns` | Space-separated regex patterns identifying test files for the test-edit denial hook. Empty disables the denial. | `"_test\.go$"` |
| `language_packs` | Space-separated list of active packs. | `go` |
| `local_hooks` | Paths to hooks *you* wrote, space-separated. Checked for existence and the execute bit like the shipped ones, run by `pre-push-check`, and — unlike editing a framework file — survives `factory upgrade`. To pass a hook its own arguments, separate entries with commas instead. | `"scripts/hooks/my-gate.sh"`, or `"scripts/hooks/my-gate.sh --strict, scripts/hooks/other.sh"` |
| `check_command` | The command that constitutes "the checks" for pre-push and diff-aware verification. | `"go test -race ./..."` |

The keys above arm the gates. The rest configure the harnesses — which model does
which work, and whether the advisory review lane runs. They used to live in a
separate `factory.config` that scripts *sourced*; they are ordinary keys here now
(Decision 41), so one file is parsed and nothing is executed.

| Key | Meaning | Example |
|---|---|---|
| `cost_profile` | `standard` (two tiers) or `economy` (adds a cheaper third for low-stakes roles). | `economy` |
| `model_provider` | Which provider seeded the tiers. `inherit` writes no pins at all. | `openrouter` |
| `<harness>_<tier>_model` | The model for that tier on that harness — `opencode`/`claude`/`codex` × `frontier`/`default`/`economy`. Blank means inherit. | `claude_economy_model: "claude-haiku-4-5"` |
| `review_lane` | `on`/`off` for the advisory adversarial PR review. Its presence is the record of having been asked. | `off` |
| `review_model` | Pin a reviewer model; blank resolves the frontier tier at run time. | `""` |
| `review_api_key_secret` | Name of the repository secret CI reads the key from. | `OPENROUTER_API_KEY` |
| `review_max_tokens` | Maximum OpenRouter completion tokens; decimal `1024`–`32768`, default `8192`. | `8192` |

Supported legacy model and review keys in `factory.config` remain readable.
Nonempty YAML values take precedence over the corresponding legacy settings.
Preview conversion with `./factory migrate-config --dry-run`.

A complete `factory.yaml` for an HTTP API project, using the template's own real keys:

```yaml
project_name: billing-api
decision_log: docs/DECISION_LOG.md
docs_root: docs
citation_prefix: ""
protected_paths: "internal/billing scripts/hooks .github/workflows"
test_file_patterns: "_test\.go$"
check_command: "go test -race ./..."
```

Set `check_command` to an application build or test command. The factory-provided `make check` invokes this value, and `./factory check` invokes `make check`. Using either as the value recurses.

A blank `citation_prefix` disables citation linting. A blank `test_file_patterns` disables test-edit denial. Other keys have their own defaults.

Two boundaries worth stating plainly:

- **`factory.yaml` is configuration, not policy.** A change that weakens what a hook enforces — removing a protected path, blanking `test_file_patterns` — is a governance change, and your review should treat it as one. The decision-log gate treats `factory.yaml` itself as governance-sensitive.
- **Empty means off.** Citations and test-edit denial are armed by configuration. You can install the commit and push gates on day one and arm the rest later.

## Language packs

The core — the Verification Contract, agent roles, harness canon and adapters, the commit/decision/push gates — is language-agnostic. Packs carry the stack opinions: one blessed stack per language, no alternatives matrix. A template that supports everything enforces nothing.

For a repository with several languages, `factory init` takes more than one pack — `--pack go,typescript` (comma-separated) or a repeated `--pack`. It asks only for the versions the selected packs need, and merges them: `test_file_patterns` becomes the union, `check_command` the packs' checks joined with `&&`, and each pack contributes configuration, dialect hooks and steps to the generated CI workflow.

| Pack | Stack | Maturity |
|---|---|---|
| Go | Ginkgo v2 + Gomega, golangci-lint, gosec, govulncheck, gremlins (mutation) | battle-tested |
| TypeScript | Vitest, Biome (format + lint), tsc, Stryker (mutation), OSV-Scanner | experimental |
| Java / Spring Boot | JUnit 5 + AssertJ + Testcontainers, Spotless (palantir), Error Prone, SpotBugs + find-sec-bugs, OSV-Scanner, PIT (mutation); Gradle or Maven | beta |

The table uses the maturity labels recorded in the [pack manifests](../packs).
The Java pack has a reported Maven adoption with project-specific adaptations.
A pack label describes the project's adoption evidence.

React and Vue projects can use the TypeScript pack. Spring Boot projects can
use the Java pack. The initializer prints stack hints from root manifests.
Add any framework-specific invariant your team needs as a custom dialect hook.

## Cost profile (opt-in)

Factory-managed invocation budgets are a separate opt-in from model tiering.
Start with `./factory budget plan --harness codex --session work --task issue-1`.
It calls no model and reports the default limits, including that execution is
disabled. Set `budget_enabled: true` in `factory.yaml` only when you want to use
`factory budget run`. See [BUDGETS.md](BUDGETS.md) for all keys and the distinction
between enforced invocation limits and client-reported cost estimates.

`factory init` asks for a cost profile alongside the model choices. `standard`
(the default) keeps two tiers — a default model for most roles, a frontier model
for `spec-writer` and `reviewer`. `economy` adds a third, cheaper tier for the
low-stakes roles (`refactorer`, `wiki-maintainer`, the opencode `small_model`),
while leaving the review path frontier.

Each harness has its own native model namespace, so each carries its own per-tier
models in `factory.yaml` — set at initialization and editable there:

```yaml
model_provider                                                              # seeds the tiers below
opencode_frontier_model / opencode_default_model / opencode_economy_model   # any provider/model
claude_frontier_model   / claude_default_model   / claude_economy_model     # Anthropic ids
codex_frontier_model    / codex_default_model    / codex_economy_model      # OpenAI ids
```

No provider is assumed. `factory init` asks once and only *seeds* these; choose
`inherit` and the factory writes no model pins at all, leaving each harness on
its own configuration. See [MODELS.md](MODELS.md).

To change a model or switch profiles, edit `factory.yaml`, run
`./factory sync all` and inspect the generated diff. The selected model and
the configured project gates can be changed independently.
[How model routing affects cost](COST_AND_TOKENS.md).

## Adopting incrementally

1. Initialize the project. Use the core-only configuration if you want to choose the gates individually. The self-test proves gates in isolated fixtures. Follow the [full pre-push setup](HOOKS.md#full-pre-push-checks) to activate local push checks while preserving existing hooks; CI runs its configured checks separately.
2. Set `protected_paths` to the directories where a silent agent change would hurt most; start narrow.
3. When you split agent roles (spec-writer vs implementer), set `test_file_patterns` to arm generator/evaluator separation.
4. When your specs cite a docs tree, set `citation_prefix` and `docs_root` to arm citation linting.

Run `./factory doctor` between each step to inspect configuration and isolated gate proofs. Its Git-hook report describes selection of the tracked `.githooks` path, whose shipped hook is direct-main-only. It does not validate arbitrary default/custom hooks or prove that a full check runs on your real pushes. Init also does not install a local `commit-msg` hook; commit lint runs through CI or `make lint-commits`.

## Upgrading

From an existing v0.1.6 project, preview the migration with the v0.2.0 installer:

```sh
curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- upgrade --ref v0.2.0 --dry-run --json
```

Inspect the proposed replacements, retirements, recovery files and blockers.
Unrecognized or customized required factory files can block automatic migration.
When the proposal is acceptable, pause factory work and confirm that same
release and proposal digest. A successful migration retains a Git-ignored local
recovery backup. The [upgrade guide](UPGRADING.md) gives the confirmation,
post-upgrade checks and rollback steps. Once on the Go factory, you can
preview a stable upgrade through the installed `./factory upgrade` command.

## Writing your own hooks

The [field-coverage hook](https://github.com/anoop2811/software-factory-template/blob/v0.2.0/docs/examples/hooks/field-coverage-check.sh) in the template source is a worked example. To adapt it to an installed project, follow the steps below and the [hook reference](HOOKS.md).

1. **Write the script** under `scripts/hooks/`. Read any project value with `. scripts/lib/config.sh` at the top, then call `factory_config_get <key>` to share project settings with the shipped shell hooks.
2. **Register it** in `factory.yaml` — `local_hooks: "scripts/hooks/my-gate.sh"` (space-separated; use commas instead when an entry carries arguments, e.g. `"scripts/hooks/my-gate.sh --strict, scripts/hooks/other.sh"`, and pre-push will run it with them) — so CI fails if the script goes missing or loses its execute bit (the fail-open edit path depends on this net). Register it there, *not* in `scripts/hooks/hook-existence-check.sh`: a local modification to that framework file can block automatic migration.
3. **Add a break/fix case** to your project's test suite: introduce the exact violation, assert the gate fires, revert, assert it passes. Factory contributors add these broader regressions to `scripts/selftest/development.sh`; public `scripts/selftest/run.sh` is a transport adapter for the fixed native core proof, not an extension file. A gate you have only watched pass proves nothing.
4. **Wire it into CI** so the gate runs on every pull request, not just locally.

For a spec to write your gates against, `specs/TEMPLATE.md` is a spec template to copy. For how the template measures agent quality end to end, see [eval/README.md](../eval/README.md).

## Running pack hooks

Execute pack hooks directly (for example, `./scripts/hooks/junit5-only-check.sh`)
or with `bash`. Do not prefix them with `sh`: it overrides the Bash shebang,
including on macOS where sh is Bash in POSIX mode. Pack dialect gates report
this invocation mistake with exit 2 before parsing their Bash-only syntax.

## Optional pack sources and selftest coverage

An installed language gate lives in `scripts/hooks/`; keep it while that pack
is configured. The upstream `packs/go/`, `packs/java/`, and `packs/typescript/`
source directories are not required in an adopted repository.
The installed CLI keeps its review workflow template at `.factory/assets/scaffold/packs/review-lane/review-pr.yml` for use when you enable the optional review lane.
Use `./factory review-lane disable` to turn off its workflow. Keep managed
assets in place so subsequent upgrades can recognize the installation.

The factory's development runner prints a named skip for each fixture group whose optional source is
missing, then continues through the remaining checks and reports the number
of skipped groups. A skip means reduced coverage, never a proven gate. Core
`scripts/`, shared libraries, and `templates/metrics.html` are still required.
Template CI exercises these omissions with
`./scripts/selftest/optional-packs.sh`; adopters run `./factory selftest`, whose
fourteen core gate pairs do not require the template's optional pack sources.

## Maven projects

`factory init --pack java` uses Maven when `pom.xml` or `mvnw` exists and
there is no `gradlew`. An executable Maven wrapper takes precedence over system
`mvn`. Use `--java-build-tool maven` or `--java-build-tool gradle` to choose
explicitly in a mixed build. No build-tool hint retains the Gradle default.

Maven installs receive native Makefile targets, Maven steps in the factory CI workflow, and
`quality-maven.xml` instead of `quality.gradle`. The initial checks run the
existing Maven verify lifecycle and the JUnit dialect gate. Merge the quality
snippet into the shared parent POM to activate Spotless, Error Prone, and
SpotBugs; PIT remains an explicit mutation command. The installer preserves
your POM. See the installed `MAVEN.md` or the template
[packs/java/maven/MAVEN.md](../packs/java/maven/MAVEN.md) for integration steps
and the Surefire `-Dtest` include-replacement trap.

The Java pack is beta based on Duke42's reported Maven adoption with local
adaptations ([issue #66](https://github.com/anoop2811/software-factory-template/issues/66)).
That evidence does not imply every Maven setup is supported.

## Optional repair loops

`factory loop` defaults to manual deterministic checks. Explicit bounded mode
requires both loop and budget opt-in, retains canonical roles, and stops for
human handoff on uncertain exit or test/governance changes. Configure a meaningful
check command and finite limits using [Loop engineering](LOOPS.md).
