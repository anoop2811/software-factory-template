# Concepts

**In short**

- The Go CLI and shared hooks can reject a specific action or artifact. The principle is due to Birgitte Böckeler (Thoughtworks).
- The Verification Contract: you may only claim what you have observed. Three claim levels, eight rules.
- Roles assign test writing, implementation and review to separate jobs. An edit hook can block the implementer from changing configured test paths.
- This file explains why the rules exist. The rulebook agents actually read is [FACTORY_RULES.md](FACTORY_RULES.md).

New to the vocabulary? See [GLOSSARY.md](GLOSSARY.md).

## Prompt, loop, harness, graph — and where the factory sits

The vocabulary around agents is a ladder, and it is worth being precise about it,
because most of it is sold as something you must go learn to *become*. A **prompt**
is a sentence — one ask, one answer. A **loop** is a cycle — an agent iterating
until a condition holds (the story loop here: spec-writer writes a red test,
implementer makes it green, reviewer checks it). A **harness** is the floor the
agent stands on — the roles, the gates, the verification contract; the part that
holds regardless of what the model improvises. A **graph** is the shape of the
work itself: nodes that do the thinking, edges that carry results between them.

The factory is the harness and the graph — not as a framework you adopt, but as
what the architecture already is:

- A **node** is a bounded job, one input and one output. That is a **role** —
  spec-writer, implementer, reviewer, refactorer, wiki-maintainer — defined once
  in the canonical opencode config and generated for every harness.
- An **edge** is a hand-off where data actually moves: not "and then", but "this
  step's output feeds that step's input." A **gate** is that edge made
  deterministic — the hook that checks what crosses and exits non-zero if it is
  wrong. Coordination is code, not a conversation.
- A **verifier** sits on an edge and tries to kill a finding before it reaches
  the answer. That is the **reviewer** role — a separate assignment from implementation. You can assign the two roles
  different models or let them inherit the harness's model.
- **Tiering models per node** — cheap for mechanical work, expensive for judgment
  — is the **economy cost profile**, routing each role to a model tier.

The factory ships plain-text [workflow recipes](WORKFLOWS.md). A recipe names
the jobs, dependencies and verifier so the harness can coordinate them using
its own tools. Resumable graph execution is [on the roadmap](FEATURE_ROADMAP.md).

## Computational controls beat inferential ones

An inferential control is a rule an agent is asked to follow: a line in a prompt, a "please always" in a system message. It works until the context window rotates it out, the model changes, or the agent decides the rule doesn't apply this time.

A computational control runs regardless: a shell script that exits nonzero, a CI job that fails the build, a pre-push hook that rejects the ref. It doesn't care whether the agent read the rule.

Design consequences:

- Every invariant worth stating gets a check. A rule in prose without a hook is a promise.
- The always-loaded context (`AGENTS.md`) stays short. Most rules don't need to be in context, because the hooks catch violations either way. Only write-time rules no hook can catch (cite the *right* line, not just a real line) belong in every session.
- Enforcement logic lives in shared scripts under `scripts/hooks/`. The opencode plugin and the Claude Code and Codex adapters are thin wrappers around the same scripts, and `shared-script-enforcement.sh` checks that this stays true.

## The Verification Contract

This is the template's central discipline. It exists because of a repeated failure: across four review rounds on the originating factory, every false "fixed/verified" claim was a check that had never been executed, and every check that actually ran held up (see `memory/lessons/001-verification-contract.md`). The fix is one sentence: **you may only claim what you have observed.**

### Three claim levels

| Level | Meaning | May say "fixed" / "verified" / "works"? |
|---|---|---|
| **WROTE** | "I wrote code intended to do X." No evidence. | No |
| **RAN** | "I executed the check for X in this session; here is the output." | Yes |
| **OBSERVED** | "I saw X happen at the level of the system the claim is about." | Yes |

RAN vs OBSERVED matters more than it looks. Testing a shell script in isolation is RAN evidence about the script — not evidence that the harness which should call the script actually calls it. "The harness blocks test edits" is only OBSERVED when you've watched the harness block a test edit.

### The eight rules

The full text is in [FACTORY_RULES.md](FACTORY_RULES.md).

| # | Rule | What it means | What enforces it |
|---|---|---|---|
| 1 | No claim without execution | Every "verified" cites the exact command run this session and pastes its output. Otherwise: "written but NOT verified". | `commit-message-lint.sh` rejects the commit |
| 2 | Verify at the level of the claim | Evidence about a component isn't evidence about the system wrapping it. Name the level you actually reached. | Discipline, plus `diff-aware-check.sh` flags stale OBSERVED claims |
| 3 | Every check must be seen to fail | break → FAIL → revert → PASS on the exact defect it exists to catch. | Break/fix fixtures in CI; the `factory-init` self-test |
| 4 | Never use an unverified API surface | A signature or config key not confirmed from current docs or source this session is a hypothesis. | Write-time rule; no hook can catch it |
| 5 | Read back state after mutating it | Confirm end state with an independent read (`git status`, `cat`), not the mutating command's own success output. | Write-time rule; no hook |
| 6 | Comments and names are not behavior | A checker satisfiable by a string in a comment checks documentation, not behavior. | Checkers strip comments; fixtures prove they still fire |
| 7 | Batch claims decompose | "All N items fixed" is N claims, each needing its own evidence line. | Lint checks each message line independently |
| 8 | Fail closed on your own uncertainty | Can't verify? Say exactly that and mark the item OPEN. | Discipline; "NOT verified" always passes the lint |

Rules 4, 5, and 8 are honest gaps: they live in the agent's write-time behavior and no hook can catch them after the fact. That's why they're in `AGENTS.md`, the always-loaded context.

### Every check must be seen to fail

Rule 3 deserves its own paragraph because it's the one that has saved us most often.

```
break the thing  ->  check FAILS  ->  revert  ->  check PASSES
```

A check you've only seen pass may be passing vacuously. We had a dead pipeline that read an empty stream, and a grep satisfied by a comment mentioning the required call — both real, both in [PATTERNS.md](PATTERNS.md). Go initialization runs the same fourteen core gate pairs as `./factory selftest`. Add a break/fix case to the project suite when you write your own gate.

### Read back state after mutating it

The canonical example: `git rm --cached` removes a file from the index, success output and all. A later `git add -A` quietly re-stages it. Confirm end state with `git ls-files` before claiming it, not with the removal command's own output.

## Generator/evaluator separation

The root cause in the originating factory's failure analysis: the same agent wrote the spec, the tests, the implementation, and the verification claims in one session. The agent that wrote the rule also judged whether the rule was followed.

The template splits the roles:

```
spec-writer ──> tests ──> implementer ──> code ──> reviewer ──> findings
                  ^            │
                  └─ edits ────┘
                     denied      (test-edit-denial.sh, exit 2)
```

| Role | Configured tier | Job |
|---|---|---|
| spec-writer | frontier | Write independent acceptance tests. |
| implementer | default | Make the tests pass. |
| refactorer | default or economy | Improve the implementation under passing tests. |
| reviewer | frontier | Inspect the change and return findings. |
| wiki-maintainer | default or economy | Update the wiki from its sources for human review. |

The test-edit hook checks the explicit `implementer` role and the project's
configured test path patterns. A missing role or empty pattern list permits the
edit. Native harness permissions also apply. Model tiers can resolve to the same
model. [Hook conditions](HOOKS.md#test-edit-denialsh).

## One canonical config, generated adapters

OpenCode provides the canonical role definitions in `.opencode/agent/*.md`.
After editing them or the model tiers in `factory.yaml`, run
`./factory sync all` and inspect the OpenCode, Claude Code and Codex configuration
diff. Run `./factory eval structural --harness=codex` to check the resulting
Codex wiring locally. The other selectors are `claude` and `opencode`.

### The two config layers

Project-specific values are split across two mechanisms, for two different reasons. It helps to see the split plainly.

- **Enforcement values are read at runtime.** `protected_paths`, `test_file_patterns`, `check_command`, `citation_prefix`, and `docs_root` live in `factory.yaml`. The hooks read them at runtime through `scripts/lib/config.sh`, so the hook scripts stay byte-identical across every adopter — nothing in them is edited at install time.
- **Identity and model values are written into harness files.** Initialization fills in project identity and generates the harness configuration. Running `./factory sync all` applies later role and model changes.

Two layers, two reasons: runtime config keeps hooks identical and upgradable; install-time substitution fills in the values the harness config can't look up for itself.

```
opencode.json (canon) ──sync──> .claude/ , .codex/ (adapters)

factory.yaml ──runtime──> hooks
agent roles  ──write-time──> hooks
CI           ──────────────> hooks
```

## Runtime configuration: factory.yaml

Project-specific values — protected paths, test file patterns, the decision-log location, the citation prefix — live in `factory.yaml` at the repo root, read by the Go runtime and through the shell hooks' `scripts/lib/config.sh` adapter. The format uses flat `key: value` entries and space-separated lists. Anything more expressive belongs in a hook.

This replaced an earlier install-time placeholder-substitution scheme, for a reason worth recording. Substituted placeholders fork every adopter from the template at install time, so upgrades required diff archaeology, and the template's own hooks couldn't run (or be tested) in the template repo itself. With runtime config, adopters can share a hook implementation. The Go upgrader checks file ownership and offers a reviewable migration with recovery. Key reference in [ADAPTING.md](ADAPTING.md).

## Decisions before code

Every decision about the factory goes in `docs/DECISION_LOG.md` before the code that implements it. `decision-log-gate.sh` requires commits touching governance-sensitive paths to reference a Decision number. Configuration isn't exempt: a `factory.yaml` change that weakens what a hook enforces is a governance change and should be reviewed as one.

## The memory loop

At session end, lessons worth keeping go to `memory/lessons/NNN-*.md` with provenance — a file and line, a fetched URL with date, or "observed YYYY-MM-DD via \<action\>". A lesson without provenance is worse than no lesson; it becomes a stale fact asserted with confidence.

A fresh Go installation seeds one lesson (`001-verification-contract.md`) to prove the pattern; adopters accumulate their own from there.

The loop closes by escalation: a per-turn reminder flag, a session-end check that records pending lessons (`loop-close-check.sh`), and a push block (`pending-lessons-push-block.sh`) that turns the ignorable nudge into a gate.
