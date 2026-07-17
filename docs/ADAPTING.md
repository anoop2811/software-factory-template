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

A complete `factory.yaml` for an HTTP API project, using the template's own real keys:

```yaml
project_name: billing-api
decision_log: docs/DECISION_LOG.md
docs_root: docs
citation_prefix: ""
protected_paths: "internal/billing scripts/hooks .github/workflows"
test_file_patterns: "_test\.go$"
check_command: "make check"
```

Every key is optional in the sense that an empty value disables the gate it arms — `citation_prefix: ""` above turns citation linting off, and a blank `test_file_patterns` would turn off the test-edit denial.

Two boundaries worth stating plainly:

- **`factory.yaml` is configuration, not policy.** A change that weakens what a hook enforces — removing a protected path, blanking `test_file_patterns` — is a governance change, and your review should treat it as one. The decision-log gate treats `factory.yaml` itself as governance-sensitive.
- **Empty means off.** Citations and test-edit denial are armed by configuration. You can install the commit and push gates on day one and arm the rest later.

## Language packs

The core — the Verification Contract, agent roles, harness canon and adapters, the commit/decision/push gates — is language-agnostic. Packs carry the stack opinions: one blessed stack per language, no alternatives matrix. A template that supports everything enforces nothing.

| Pack | Stack | Maturity |
|---|---|---|
| Go | Ginkgo v2 + Gomega, golangci-lint, gosec, govulncheck, gremlins (mutation) | battle-tested |
| TypeScript | Vitest, ESLint flat config, Stryker (mutation) | experimental |
| Java / Spring Boot | JUnit 5 + AssertJ + Testcontainers, Spotless (palantir), Error Prone, SpotBugs + find-sec-bugs, OSV-Scanner, PIT (mutation) | experimental |

The labels mean:

- **battle-tested** — a real project shipped under this pack. The Go pack built a production service; its hooks (`ginkgo-only-check.sh`, for one) have caught real drift.
- **beta** — at least one real repository has adopted it, nothing has shipped under it yet.
- **experimental** — no proven adopter yet. The stack may ship complete — Java's does, with CI, formatter, and gates — but the label tracks adoption, not completeness. Treat its choices as proposals.

A label changes only on evidence: a pack moves up when a real project adopts it, not when its fixtures pass.

## Adopting incrementally

1. Run `factory-init.sh`; leave `citation_prefix` and `test_file_patterns` empty for now — the commit and push gates the self-test just proved are already doing work.
2. Set `protected_paths` to the directories where a silent agent change would hurt most; start narrow.
3. When you split agent roles (spec-writer vs implementer), set `test_file_patterns` to arm generator/evaluator separation.
4. When your specs cite a docs tree, set `citation_prefix` and `docs_root` to arm citation linting.

## Upgrading

Hooks read `factory.yaml` instead of containing substituted values, so an upgrade is: copy the new `scripts/hooks/`, `scripts/lib/`, and workflow files over yours, review the diff (they're governance-sensitive paths — the decision-log gate will remind you), and re-run the break/fix self-test. Your `factory.yaml` is untouched.

## Writing your own hooks

A new gate is four steps. `docs/examples/hooks/field-coverage-check.sh` is the worked example of the whole pattern; see [HOOKS.md](HOOKS.md).

1. **Write the script** under `scripts/hooks/`. Read any project value with `. scripts/lib/config.sh` at the top, then `factory_config_get <key>` — the same way every shipped hook reads `factory.yaml`, so your gate stays byte-identical across adopters too.
2. **Register it** in `scripts/hooks/hook-existence-check.sh`, so CI fails if the script goes missing or loses its execute bit (the fail-open edit path depends on this net).
3. **Add a break/fix case** to `scripts/selftest/run.sh`: introduce the exact violation, assert the gate fires, revert, assert it passes. A gate you have only watched pass proves nothing.
4. **Wire it into CI** so the gate runs on every pull request, not just locally.

For a spec to write your gates against, `specs/TEMPLATE.md` is a spec template to copy. For how the template measures agent quality end to end, see [eval/README.md](../eval/README.md).
