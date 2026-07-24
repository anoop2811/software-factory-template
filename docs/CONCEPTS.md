# Concepts

**In short**

- Controls live in shell hooks that block, not prompts that ask. The principle is due to Birgitte Böckeler (Thoughtworks).
- The Verification Contract: you may only claim what you have observed. Three claim levels, eight rules.
- Roles are split so the agent that writes tests never writes implementation — and the split is hook-enforced.
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
  the answer. That is the **reviewer** role — adversarial by design, and never
  the same model that wrote the code (generator/evaluator separation).
- **Tiering models per node** — cheap for mechanical work, expensive for judgment
  — is the **economy cost profile**, routing each role to a model tier.

So "graph engineering" is not a new discipline to bolt on. The factory is already
a graph: roles are the nodes, gates are the verified edges, the reviewer is the
skeptic, models tier by role — and it runs the same on Claude Code, opencode, and
Codex because the roles are canonical. What a workflow *recipe* adds, when you
want it, is making one specific composition explicit and lintable; the substrate
is the roles and gates you already have.

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

A check you've only seen pass may be passing vacuously. We had a dead pipeline that read an empty stream, and a grep satisfied by a comment mentioning the required call — both real, both in [PATTERNS.md](PATTERNS.md). So every hook ships with a break/fix fixture, and `factory-init` proves the installed gates fire before declaring adoption successful.

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

| Role | Model | Constraint |
|---|---|---|
| spec-writer | frontier | writes acceptance tests only, never implementation |
| implementer | default | makes failing tests pass; cannot edit test files |
| refactorer | default | cleans up under green tests; also cannot edit tests |
| reviewer | frontier, never the code's author | edit permission denied entirely; output is findings for a human |
| wiki-maintainer | — | reads immutable sources, writes the wiki layer; bash denied, every edit lands as a reviewed PR |

The separation is only real because it's hook-enforced. Telling a session "you are the implementer, don't touch tests" is an inferential control; `test-edit-denial.sh` in the write path is the computational one.

## One canonical config, generated adapters

opencode is the canonical harness. `opencode.json` and `.opencode/agent/*.md` are the source of truth; `scripts/sync-claude.sh` and `scripts/sync-codex.sh` generate the Claude Code and Codex configurations. CI runs the sync and fails if the committed files drift, so a hand-edited adapter can't ship.

### The two config layers

Project-specific values are split across two mechanisms, for two different reasons. It helps to see the split plainly.

- **Enforcement values are read at runtime.** `protected_paths`, `test_file_patterns`, `check_command`, `citation_prefix`, and `docs_root` live in `factory.yaml`. The hooks read them at runtime through `scripts/lib/config.sh`, so the hook scripts stay byte-identical across every adopter — nothing in them is edited at install time.
- **Identity and model values are substituted once.** The project name, the GitHub owner, and the model strings in `opencode.json` and the agent `.md` files are substituted a single time, at install, by `factory-init`. This is not a stylistic choice: `opencode.json` can't read `factory.yaml`, so the harness config has no way to pull those values at runtime. They have to be baked in when the files are copied.

Two layers, two reasons: runtime config keeps hooks identical and upgradable; install-time substitution fills in the values the harness config can't look up for itself.

```
opencode.json (canon) ──sync──> .claude/ , .codex/ (adapters)

factory.yaml ──runtime──> hooks
agent roles  ──write-time──> hooks
CI           ──────────────> hooks
```

## Runtime configuration: factory.yaml

Project-specific values — protected paths, test file patterns, the decision-log location, the citation prefix — live in `factory.yaml` at the repo root, read at runtime by `scripts/lib/config.sh`. The format is deliberately constrained (flat `key: value`, space-separated lists, no nesting) so the parser stays a few lines of shell. Anything more expressive belongs in a hook.

This replaced an earlier install-time placeholder-substitution scheme, for a reason worth recording. Substituted placeholders fork every adopter from the template at install time, so upgrades required diff archaeology, and the template's own hooks couldn't run (or be tested) in the template repo itself. With runtime config, hook files stay byte-identical between template and adopters, upgrades are file copies, and the template dogfoods its own gates. Key reference in [ADAPTING.md](ADAPTING.md).

## Decisions before code

Every decision about the factory goes in `docs/DECISION_LOG.md` before the code that implements it. `decision-log-gate.sh` requires commits touching governance-sensitive paths to reference a Decision number. Configuration isn't exempt: a `factory.yaml` change that weakens what a hook enforces is a governance change and should be reviewed as one.

## The memory loop

At session end, lessons worth keeping go to `memory/lessons/NNN-*.md` with provenance — a file and line, a fetched URL with date, or "observed YYYY-MM-DD via \<action\>". A lesson without provenance is worse than no lesson; it becomes a stale fact asserted with confidence.

The template ships exactly one lesson (`001-verification-contract.md`) to prove the pattern; adopters accumulate their own from there.

The loop closes by escalation: a per-turn reminder flag, a session-end check that records pending lessons (`loop-close-check.sh`), and a push block (`pending-lessons-push-block.sh`) that turns the ignorable nudge into a gate.
