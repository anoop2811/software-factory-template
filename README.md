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
```

The installer only fetches: it clones the template into `~/.software-factory-template` and stops. It runs nothing it downloaded. Prefer to inspect first?

```bash
curl -fsSLO https://softwareaifactory.sh/install.sh && less install.sh && sh install.sh
```

`factory-init.sh` detects your stack, writes `factory.yaml`, installs the hooks, then runs a break/fix self-test so you watch a gate fire before it declares success. Afterward, try pushing straight to `main`, or committing with the message `fix: verified the thing`. Both get blocked. That's the product.

## Getting started

1. Read [docs/CONCEPTS.md](docs/CONCEPTS.md) (~5 min) — why the gates exist.
2. Run `factory-init` in your repo to install the hooks and prove they fire.
3. Read [docs/ADAPTING.md](docs/ADAPTING.md) "Adopting incrementally" to arm gates one at a time.
4. Open [docs/HOOKS.md](docs/HOOKS.md) when a gate blocks you and you want to know why.

## The idea

A prompt that says "never edit test files" is a request an agent may or may not honor. A shell hook that exits 2 on the edit is a fact. Put the control in a hook, not in the prompt — that principle is due to Birgitte Böckeler (Thoughtworks), and this template applies it to commit discipline, push gates, role separation, and citation integrity.

The second idea is the Verification Contract: only claim what you've observed. Three claim levels (WROTE / RAN / OBSERVED), every check proven by watching it fail. See [docs/CONCEPTS.md](docs/CONCEPTS.md).

## What's inside

| Piece | Where | What it does |
|---|---|---|
| Agent roles | `.opencode/agent/` | spec-writer, implementer, refactorer, reviewer, wiki-maintainer. The role that writes tests never writes implementation — enforced by a hook, not by trust. |
| Hooks | `scripts/hooks/` | Deterministic gates on commits, pushes, and CI. Each one in [docs/HOOKS.md](docs/HOOKS.md). |
| Harness adapters | `scripts/sync-*.sh` | opencode is canonical; the Claude Code and Codex configs are generated from it, and CI fails on drift. |
| Language packs | `packs/` | The core never mentions a language. Packs carry the stack opinions. |

## Pack maturity

| Pack | Stack | Maturity |
|---|---|---|
| Go | Ginkgo v2 + Gomega, golangci-lint, gosec, govulncheck, gremlins | battle-tested |
| TypeScript | Vitest, ESLint flat config, Stryker | experimental |
| Java / Spring Boot | JUnit 5 + AssertJ, Checkstyle + ErrorProne, PIT | experimental |

Labels are evidence, not roadmap: battle-tested means a real project shipped under the pack, beta means at least one real repo adopted it, experimental means only fixtures exist.

## Documentation

- [docs/CONCEPTS.md](docs/CONCEPTS.md) — why the rules exist, including the full Verification Contract
- [docs/ADAPTING.md](docs/ADAPTING.md) — adopting the factory in an existing project
- [docs/HOOKS.md](docs/HOOKS.md) — every hook: when it fires, exit codes, what a failure looks like
- [docs/PATTERNS.md](docs/PATTERNS.md) — failure patterns we hit in practice, and the fixes
- [docs/FACTORY_RULES.md](docs/FACTORY_RULES.md) — the operational rulebook agents read
- [docs/GLOSSARY.md](docs/GLOSSARY.md) — terms defined
- [specs/TEMPLATE.md](specs/TEMPLATE.md) — a spec template to copy
- [eval/README.md](eval/README.md) — how the template evaluates agent quality

## Contributing

Contributions are welcome. The template runs its own factory, so the gates are
hooks that reject a bad commit or push, not conventions to remember.

- [CONTRIBUTING.md](CONTRIBUTING.md) — the workflow, the gates, and the break/fix fixture rule
- [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) — the standards we hold each other to
- [SECURITY.md](SECURITY.md) — reporting vulnerabilities privately

New to the codebase? Start with [docs/GLOSSARY.md](docs/GLOSSARY.md) and
[docs/CONCEPTS.md](docs/CONCEPTS.md). Bug reports and feature requests have issue
templates; PRs have a checklist tied to the gates above.

## License

MIT. See [LICENSE](LICENSE).
