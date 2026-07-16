# Factory Rules — Computational Controls and Their Enforcement

**In short**

- Every spec invariant gets a computational check. Prose without a check is a promise, not a control.
- This is the full reference, read on demand when working on the factory itself. Write-time rules live in `AGENTS.md`, which is always in context.
- The Verification Contract, in one sentence: you may only claim what you have observed.

## Principle

If you write "every field participates in the derived value" in a spec, you also write the check that verifies it. If an ADR says "enforcement is shared between harnesses", a check verifies that too. The check is the invariant; the prose is the documentation.

## The hooks

Per-hook detail — fire conditions, exit codes, failure output — is in [HOOKS.md](HOOKS.md). The map:

| Hook | Enforces | Runs in |
|---|---|---|
| `scripts/citation-lint.sh` | Every `<citation_prefix>*.md:NN` citation resolves against `docs_root`: file exists, line in range. Empty prefix disables. | CI, every commit |
| `scripts/hooks/commit-message-lint.sh` | Conventional commits, max 6 bullets of 25 words, no bare "verified"/"fixed"/"works" without command+output. | CI on PR commits; `make lint-commits` |
| `scripts/hooks/decision-log-gate.sh` | Commits touching governance paths reference a Decision number. | CI, pre-push |
| `scripts/hooks/diff-aware-check.sh` | Changed files trigger their mapped re-verification. | CI, pre-push, `make diff-aware` |
| `scripts/hooks/direct-main-push-block.sh` | No local push updates `refs/heads/main`. | tracked pre-push hook |
| `scripts/hooks/test-edit-denial.sh` | The implementer role can't edit test files. | agent write path (pre-tool-use), not CI |
| `scripts/hooks/shared-script-enforcement.sh` | Adapters call the shared scripts instead of reimplementing enforcement inline. | CI, `make check` |
| `scripts/hooks/hook-existence-check.sh` | Every registered hook exists and is executable (the fail-open safety net). | CI, `make check` |
| `scripts/hooks/pending-lessons-push-block.sh` | No push while a lesson reminder or staleness flag is unaddressed. | pre-push, CI |
| `scripts/hooks/loop-close-check.sh` | Nothing — a session-end nudge that writes `memory/PENDING-LESSONS.md`. | opencode `dispose` hook |
| `packs/go/hooks/ginkgo-only-check.sh` | (Go pack) one testing dialect: Ginkgo v2 + Gomega, stdlib `testing` only in the `RunSpecs` bootstrap. | CI, `make check` |
| `docs/examples/hooks/field-coverage-check.sh` | Example only, not an active gate: a struct's coverage function includes every non-excluded field. Copy and adapt. | nothing by default |

Boundaries worth remembering:

- `citation-lint.sh` checks that a cited line exists, not that it's the *right* line for the claim. That part is a write-time rule in `AGENTS.md`.
- `test-edit-denial.sh` denies only when `FACTORY_AGENT_ROLE` is explicitly `implementer`. Unset role allows — a missing env var must not block the spec-writer, whose job is writing tests.
- `direct-main-push-block.sh` is local enforcement with repo permissions, not server-side protection. A human still reviews and merges the pull request.

## Structural rules

**Generator/evaluator separation.** The agent that writes the spec doesn't write the implementation; the agent that writes the tests doesn't write the implementation; the agent that writes the implementation doesn't write the tests. Enforced by `test-edit-denial.sh`, by role separation in the agent definitions (spec-writer = frontier model, implementer = default model), and by `FACTORY_AGENT_ROLE` set explicitly in each agent's environment.

**Go tests use Ginkgo v2 and Gomega only.** The `testing` package exists only in each package's `RunSpecs` bootstrap; specs use Ginkgo nodes, parameterized cases use `DescribeTable`, assertions use Gomega. Enforced by `ginkgo-only-check.sh`.

**Enforcement logic is shared, not inline.** Deterministic controls live in `scripts/hooks/`; opencode, Claude Code, and Codex call the same scripts. Enforced by `shared-script-enforcement.sh`.

**Derived-value functions cover every field.** If a field is added to the struct and not to the coverage function, CI fails. Enforced by your adaptation of `docs/examples/hooks/field-coverage-check.sh`, pinned by a golden vector test — hardcode one known output as a constant so any change to the computation breaks the build.

**go.sum is tracked.** The dependency hash-lock is committed, not gitignored, and CI tool versions are pinned, never `@latest`.

**CODEOWNERS enforces protected paths.** `__PROTECTED_PATH__/` gets a CODEOWNERS rule requiring review, and branch protection requires that review before merge. This one is platform-enforced, not hook-enforced — a hook that exits 0 either way is decorative, and the decorative version was removed.

## What belongs in AGENTS.md vs this file

- **AGENTS.md (always in context):** project overview, where things live, and the write-time rules no hook can catch — cite by file:line, verified claims cite exact output, protected-path review discipline, the role env var. Short.
- **This file (read on demand):** the hook reference, the structural rules, the enforcement mechanisms. Read when working on the factory itself.

If a check is computational, its prose rule doesn't need to be in every session's context; the hook catches the violation regardless.

## The Verification Contract

This contract exists because across four review rounds on the original factory, every false "fixed/verified" claim was a check that had not been executed, and every check that was actually executed held up. You may only claim what you have observed.

### Three levels — always say which one you're at

| Level | Meaning | May use "fixed"/"verified"/"works"? |
|---|---|---|
| **WROTE** | "I wrote code intended to do X" (no evidence) | No |
| **RAN** | "I executed the check for X in this session; here is the output" | Yes |
| **OBSERVED** | "I saw X happen at the system level the claim is about" | Yes |

### The eight rules

1. **No claim without execution.** Every "verified" line cites the exact command run in THIS session and pastes its real output. Didn't run it? Write "written but NOT verified" — always acceptable. A false "verified" never is.
2. **Verify at the level of the claim.** Testing a component doesn't verify the system wrapping it. "The harness blocks test edits" requires watching the harness block a test edit; a run of the shell script it calls is evidence about the script. Name the level your evidence actually reaches.
3. **Every check must be seen to fail.** break → FAIL → revert → PASS on the exact defect it exists to catch. A check only seen passing may be passing vacuously (dead pipeline, satisfied by a comment — both happened; see [PATTERNS.md](PATTERNS.md)). This is TDD's red phase applied to enforcement.
4. **Never use an unverified API surface.** A signature, option, config key, or event shape not confirmed from current docs, types, or source in this session is a hypothesis. And after confirming the types, still run the code — types don't prove runtime behavior.
5. **Read back state after mutating it.** Confirm the end state with an independent read (`git ls-files`, `git status`, `cat`) before claiming it. A later command can silently undo what the success message described.
6. **Comments and names are not behavior.** Never write a comment describing what code does in place of making the code do it. Never let a checker be satisfiable by strings or comments.
7. **Batch claims decompose.** "All N items fixed" is N independent claims, each needing its own evidence line. Verified items must not lend credibility to unverified ones in the same list.
8. **Fail closed on your own uncertainty.** Can't verify — no access, missing tool, can't run the harness? Say exactly that and mark the item OPEN. An honest "unverified" costs one review cycle; a false "verified" costs trust and ships the defect.

### Pre-send gate

Before sending a completion message, run this over your own draft: for each claim, did I watch this happen, or do I merely expect it? Rewrite every expectation as evidence or an explicit OPEN item.

### The ratchet

Prose rules decay; hooks don't. Where the contract can be enforced computationally, it is: `commit-message-lint.sh` rejects "verified"/"fixed"/"works" lines lacking a command+output citation. The contract survives model switching because it lives in committed files and hooks, not in any model's memory.
