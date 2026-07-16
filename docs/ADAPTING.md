# Adapting the factory to your project

**In short**

- Built for brownfield adoption: existing repo, existing tests, existing conventions.
- All project-specific values go in one `factory.yaml`; hook files stay byte-identical to the template, so upgrades are file copies.
- Several gates are armed by configuration — empty means off. Adopt incrementally.
- `factory-init.sh` won't declare success until a break/fix self-test has watched the installed gates fire.

## factory-init

`factory-init.sh` installs the factory into an existing project. Four steps, in order:

1. **Detects the stack.** Inspects the repo (go.mod, package.json, pom.xml/build.gradle) and proposes a language pack, or none.
2. **Writes `factory.yaml`.** Everything project-specific goes in the config file; nothing in the hooks themselves is edited.
3. **Installs the hooks.** Shared scripts into `scripts/hooks/` and `scripts/lib/`, the git pre-push entry point, the CI workflow, the agent roles, and the harness adapters (opencode canonical; Claude Code and Codex generated).
4. **Proves the gates fire.** A break/fix self-test violates each installed gate — a malformed commit message, a staged direct-to-main push, an implementer-role test edit — confirms the block, reverts, and confirms the pass.

If the self-test fails, the installation is not successful, and factory-init says so.

## factory.yaml reference

Read at runtime by `scripts/lib/config.sh`. Format: flat `key: value` pairs, no nesting; lists are space-separated on one line; values may be double-quoted; a trailing `# comment` is stripped. Anything more expressive belongs in a hook, not in configuration.

| Key | Meaning | Example |
|---|---|---|
| `project_name` | Human-readable name, used in agent role prompts and messages. | `myproject` |
| `decision_log` | Path to the decision log that `decision-log-gate.sh` checks references against. | `docs/DECISION_LOG.md` |
| `docs_root` | Root of the spec/docs tree that citations resolve against. | `docs` |
| `citation_prefix` | Prefix for `PREFIX_*.md:NN` citations checked by `citation-lint.sh`. Empty disables the check. | `MYPROJECT_` |
| `protected_paths` | Space-separated path prefixes treated as governance-sensitive: commits touching them must reference a Decision. | `"internal/billing scripts/hooks"` |
| `test_file_patterns` | Space-separated regex patterns identifying test files for the test-edit denial hook. Empty disables the denial. | `"_test\.go$"` |
| `language_packs` | Space-separated list of active packs. | `go` |
| `check_command` | The command that constitutes "the checks" for pre-push and diff-aware verification. | `"make check"` |

Two boundaries worth stating plainly:

- **`factory.yaml` is configuration, not policy.** A change that weakens what a hook enforces — removing a protected path, blanking `test_file_patterns` — is a governance change, and your review should treat it as one. The decision-log gate treats `factory.yaml` itself as governance-sensitive.
- **Empty means off.** Citations and test-edit denial are armed by configuration. You can install the commit and push gates on day one and arm the rest later.

## Language packs

The core — the Verification Contract, agent roles, harness canon and adapters, the commit/decision/push gates — is language-agnostic. Packs carry the stack opinions: one blessed stack per language, no alternatives matrix. A template that supports everything enforces nothing.

| Pack | Stack | Maturity |
|---|---|---|
| Go | Ginkgo v2 + Gomega, golangci-lint, gosec, govulncheck, gremlins (mutation) | battle-tested |
| TypeScript | Vitest, ESLint flat config, Stryker (mutation) | experimental |
| Java / Spring Boot | JUnit 5 + AssertJ, Checkstyle + ErrorProne, PIT (mutation) | experimental |

The labels mean:

- **battle-tested** — a real project shipped under this pack. The Go pack built the originating factory's product; its hooks (`ginkgo-only-check.sh`, for one) have caught real drift.
- **beta** — at least one real repository has adopted it, nothing has shipped under it yet.
- **experimental** — fixtures only, no real adopter. Treat its choices as proposals.

A label changes only on evidence: a pack moves up when a real project adopts it, not when its fixtures pass.

## Adopting incrementally

1. Run `factory-init.sh`; leave `citation_prefix` and `test_file_patterns` empty for now — the commit and push gates the self-test just proved are already doing work.
2. Set `protected_paths` to the directories where a silent agent change would hurt most; start narrow.
3. When you split agent roles (spec-writer vs implementer), set `test_file_patterns` to arm generator/evaluator separation.
4. When your specs cite a docs tree, set `citation_prefix` and `docs_root` to arm citation linting.

## Upgrading

Hooks read `factory.yaml` instead of containing substituted values, so an upgrade is: copy the new `scripts/hooks/`, `scripts/lib/`, and workflow files over yours, review the diff (they're governance-sensitive paths — the decision-log gate will remind you), and re-run the break/fix self-test. Your `factory.yaml` is untouched.

## Writing your own hooks

Drop a script in `scripts/hooks/`, register it in `hook-existence-check.sh`, and ship it with a break/fix fixture proving it catches the defect it exists for. `docs/examples/hooks/field-coverage-check.sh` is the worked example of the pattern; see [HOOKS.md](HOOKS.md).
