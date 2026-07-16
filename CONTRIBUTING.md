# Contributing

**In short**

- The template runs its own factory. The gates below are hooks that will reject your commit or push, not conventions to remember.
- Decisions go in `docs/DECISION_LOG.md` before the code that implements them.
- Every new or changed hook ships with a break/fix fixture. No fixture, no merge.
- Direct pushes to `main` are blocked. Push a feature branch, open a PR.

## Decisions before code

Every change to what the template enforces — a new hook, a config key, a pack, a moved boundary — gets an entry in `docs/DECISION_LOG.md` first. One entry per decision: what, why, provenance.

`decision-log-gate.sh` requires commits touching governance-sensitive paths (`scripts/`, `.github/workflows/`, `opencode.json`, the adapters, `factory.yaml`, and everything in `protected_paths`) to reference a Decision number. Unsure whether your change is a "decision"? If it changes what a hook enforces, what an agent may do, or what an adopter receives, it is.

## Commit messages

`commit-message-lint.sh` checks every commit in a PR:

| Check | Rule |
|---|---|
| Subject | Conventional commits: `<type>: <description>`, type one of `feat` `fix` `chore` `docs` `refactor` `test` `ci` `build` `perf`. Optional scope. No trailing period. |
| Body | At most 6 bullets, each at most 25 words. |
| Claims | Any line saying "verified", "fixed", or "works" must cite the command run and its output. |

If you didn't run the check, write "written but NOT verified". That phrase is always acceptable. A false "verified" never is.

A body bullet that passes:

```
- verified: ./scripts/hooks/commit-message-lint.sh HEAD -> "commit-message-lint: OK"
```

## Every hook ships with its break/fix fixture

A hook that has only been seen to pass may be structurally incapable of failing. We shipped a checker once that was satisfied by a comment mentioning the required call, and another that was a dead `grep -q | grep` pipeline — the fixture rule is how both died (see [docs/PATTERNS.md](docs/PATTERNS.md)). A PR adding or changing a hook must include the fixture that proves it:

1. Introduce the exact defect the hook exists to catch.
2. Assert the hook exits nonzero.
3. Revert the defect.
4. Assert the hook passes.

CI runs the fixtures. A hook PR without one doesn't merge, no matter how obviously correct the hook reads. Also register the new hook in `scripts/hooks/hook-existence-check.sh` so the fail-open safety net covers it.

## Workflow

```bash
./scripts/prereq-check.sh      # verify required tools
make check                     # lint, tests, and all CI hooks locally
make lint-commits              # lint HEAD's commit message
./scripts/pre-push-check.sh    # the full pre-push gate
```

If you change `opencode.json` or the agent role files, run `make sync-harnesses` and commit the regenerated `.claude/` and `.codex/` files. CI fails on adapter drift.

## Style

- No emojis in files.
- No comments in code unless they carry information the code can't (a citation, a non-obvious constraint).
- Plain prose in docs, no hype. Doc claims follow the same Verification Contract as commit messages: state what was observed, label what wasn't.

## Reporting problems

Bug reports with a failing reproduction get acted on fastest. Break/fix form is ideal: here's the state, here's the command, here's the output showing the gate misfiring or failing to fire. Security issues go through [SECURITY.md](SECURITY.md) — don't open a public issue.
