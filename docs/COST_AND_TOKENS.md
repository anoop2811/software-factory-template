# Cost and tokens

You can choose a model tier for each role, limit factory-managed
agent launches and inspect the usage the harness reported. Choose the control
that matches the question:

| What do you want to know or change? | Where to start |
|---|---|
| Which model will a role use? | Edit the tiers in `factory.yaml` and run `./factory sync all`. Blank tiers inherit the harness model. |
| May the factory launch an agent for this task? | [Budget plan](BUDGETS.md) explains admission, including disabled or exhausted budgets. Agent launches are opt-in. |
| What did those launches consume? | `./factory budget report --json` reads the local ledger. Missing usage stays unknown and cost figures are estimates. |
| When should the factory stop trying to repair a change? | [Bounded loops](LOOPS.md) share the invocation budget across implementation and review. |
| Which gates blocked work? | `./factory report` shows recorded blocks and a separately labelled estimate. |

The installed core self-test runs without a model. A configured project check,
coding harness or enabled PR review can have its own costs.

## The two cost surfaces

**1. The cost of using the factory.** The overhead the factory imposes on top of
the raw task: governance context loaded into the prompt (`AGENTS.md`,
`FACTORY_RULES.md`, role prompts, the Verification Contract), plus the extra
turns discipline adds — a spec pass, a test pass, a reviewer pass. Charges depend on the harness, task and provider's caching behavior (see L3).

**2. The cost of the work done with the factory.** The tokens the actual
implementation burns — exploration, generation, rework. This is where the money
is, and it is mostly a function of which model does the work and how much it
flails. Specs and deterministic feedback can surface errors before another model call.
A configured tier can route high-volume work to a cheaper model (L2).

A cost plan has to help both, and be honest that surface 2 is the larger prize.

## Where the factory spends tokens today

- **Always-loaded context.** Instruction files can become part of a harness's model context.
  Their cost depends on how the harness assembles requests and how the provider handles cached input.
- **The standard profile uses two tiers.** `spec-writer` and
  `reviewer` run on the frontier tier; `implementer`, `refactorer`, and
  `wiki-maintainer` run on the default tier. Both model strings are chosen at
  `factory init` and recorded in `factory.yaml`, then substituted into the
  harness configs (`opencode.json` and the agent frontmatter) by
  `./factory sync all`. Since Decision 41 that is the same file the gates read:
  routing and enforcement are different *keys*, not different files, and routing
  still relaxes no gate.
- **`small_model` now follows the economy tier.** The opencode lightweight-task
  model (titles, summaries) was pointed at the default tier; under the `economy`
  profile it routes to the cheaper economy tier instead. (This lever is
  opencode-specific — Claude and Codex have no equivalent per-project
  small-model knob, but they still get the per-role economy routing below.)
- **Local gate proofs.** `./factory selftest` and `./factory doctor` run their
  core gate proofs without a model. Application commands you configure can
  invoke additional tools.

## The levers

### L1 — Deterministic enforcement

The core principle (put the control in a hook, not the prompt) is also a token
principle. A hook that exits non-zero costs zero model tokens; the same rule
enforced by asking a model "did it edit a test file? did it fake a verified
claim?" costs tokens on every review. The more enforcement lives in hooks, the
less the system prompt has to carry rules the model re-reads every turn. This is
already true today — the plan is to keep choosing it, and to resist adding
LLM-judge checks where a deterministic one would do.

### L2 — Model-tier routing per role (the big lever)

Match the model to the role's volume and stakes:

| Role | Volume | Stakes | Tier |
|---|---|---|---|
| `spec-writer` | low | high (spec correctness) | frontier |
| `reviewer` | low | high (adversarial catch) | frontier — never downgrade the adversary |
| `implementer` | high | medium (tests are the net) | default in both shipped profiles |
| `refactorer` | high | low (behaviour-preserving) | default under standard, economy under economy |
| `wiki-maintainer` | medium | low | default under standard, economy under economy |

The insight: once the spec and tests exist, the implementer's job is bounded and
checkable — a cheaper model that passes the gates is as good as an expensive one
that passes the gates. The correctness-critical roles are low-volume, so keeping
them on a frontier model is cheap. This is where a three-tier profile earns its
keep.

### L3 — A cache-friendly stable prefix

The factory's instruction files do not change turn to turn, which is the ideal
shape for prompt caching. If the harness caches the stable prefix, the governance
overhead may decrease on cache hits, subject to provider pricing. The design value:
keep the always-loaded context stable and ordered — governance, then role prompt,
then the volatile task and diff — so the cacheable part stays contiguous and does
not get invalidated by interleaving volatile content into it.

### L4 — Context budget and lazy loading

`AGENTS.md` already defers detail ("read `FACTORY_RULES.md` when working on the
factory itself, not when writing product code"). Extend that discipline: keep the
always-on set minimal and push detail behind on-demand reads. A periodic audit of
what is always-loaded versus lazy is a direct token lever, and `wiki-lint`'s
staleness mode already exists to keep the wiki and lessons from growing into
unbounded per-session context.

### L5 — Fail fast, check only the diff

Deterministic gates catch errors in the cheapest possible loop — a shell exit
code — before the agent generates downstream work you throw away. Diff-scoped
checks run on what changed rather than re-reading the tree. Local pre-push gates
mean a mechanical miss is caught before it costs a model-plus-CI cycle.

## The cost-optimized profile (opt-in)

A `cost_profile`, chosen at `factory init` and recorded in `factory.yaml`
alongside the models, defaulting to `standard`:

- **`standard`** — the economy-eligible roles collapse to each harness's default
  model, so there is no third tier and nothing to think about.
- **`economy`** — turns on a third, cheaper tier and routes the high-volume,
  low-stakes roles to it: `refactorer`, `wiki-maintainer` and the opencode
  `small_model`. Moving the implementer to that tier is a proposal.
  `spec-writer` and `reviewer` stay frontier; the profile never downgrades the
  roles whose job is to catch mistakes.

### Per-harness models

v0.2.0 seeds the following strings when the operator selects OpenRouter
during initialization. They are configurable defaults. Check the selections
against model availability for your account. Choosing `inherit` leaves the
model tiers blank.

| Tier | opencode (OpenRouter) | Codex | Claude |
|---|---|---|---|
| frontier | `openrouter/z-ai/glm-5.2` | `gpt-5.6-sol` | `claude-opus-4-8` |
| default | `openrouter/z-ai/glm-5.2` | `gpt-5.6-terra` | `claude-sonnet-4-6` |
| economy | `openrouter/qwen/qwen3-coder` | `gpt-5.6-luna` | `claude-haiku-4-5` |

```yaml
# factory.yaml (excerpt)
cost_profile: "economy"
opencode_frontier_model: "openrouter/z-ai/glm-5.2"
opencode_default_model: "openrouter/z-ai/glm-5.2"
opencode_economy_model: "openrouter/qwen/qwen3-coder"
claude_frontier_model: "claude-opus-4-8"
claude_default_model: "claude-sonnet-4-6"
claude_economy_model: "claude-haiku-4-5"
codex_frontier_model: "gpt-5.6-sol"
codex_default_model: "gpt-5.6-terra"
codex_economy_model: "gpt-5.6-luna"
```

The Go synchronizer resolves the tier from the role and the profile.
Spec-writer and reviewer use frontier. Refactorer and wiki-maintainer use
economy when selected, otherwise default. Implementer uses default.
Each adapter writes the resulting model in the format its harness expects.

### Changing it later

Reconfiguring is one edit and one command — no re-init:

```sh
# change a model, or flip cost_profile between standard and economy, in factory.yaml
$ ./factory sync all
```

`factory.yaml` is the single source of truth for every harness's models,
including OpenCode. After reviewing the diff, commit the generated changes
along with the configuration change. Fresh initialization also runs the sync.

The profile is a routing change only. It arms no new behaviour and relaxes no
gate — the same hooks fire regardless of which model did the work. That is the
point: the gates are what let you run a cheaper model safely, because they check
the output rather than trusting the author.

## Downgrades are earned, not assumed

A role's model tier drops only when the eval harness shows the cheaper model
still passes the gates on that role's work — the same evidence rule the pack
maturity labels follow. "Cheaper" is a hypothesis until the fixtures pass under
it; a model that saves tokens per turn but fails the gates and triggers retries
costs *more*, not less. So the `economy` implementer is gated on a measured pass,
not on a hunch. This keeps cost tied to the factory's existing culture: claims
are earned by watching something succeed, not asserted.

## Measurement

Decision 46 adds an opt-in exception to the reporting boundary below:
`factory budget` supervises invocations it launches and reads their documented
structured usage events. It does not scrape unrelated sessions or add metering
to interactive runs. Reported tokens, client-side USD estimates, unknown usage,
and observed wall time stay distinct. See [BUDGETS.md](BUDGETS.md) for limits and
cross-harness evidence. The existing `factory report` remains a gate report;
`factory budget report --json` exports the separate local invocation ledger.

For other interactive work, use your harness or provider's own usage records.
The factory ledger covers only the invocations the factory launches.

### A post-session report: `factory report` (shipped)

`factory report` shows what the factory did, in three registers kept visibly
separate, so each number means exactly what it says:

- **Facts the factory computes itself (no counterfactual).** The deterministic
  shipped gate entrypoints it counts, the cost profile and
  model tiers, and the gate *blocks* recorded — each one a catch a hook made,
  logged by `scripts/lib/events.sh` when it fired. A hook that exits non-zero
  genuinely cost no model tokens; this is what *happened*, not what was avoided.
  The block window resets with `factory report --clear`.
- **Measured token spend.** Use `./factory budget report` for reported usage
  from factory-managed runs. The gate report does not read interactive sessions.
- **One clearly-labeled estimate.** The estimate assumes a hypothetical review pass
  per recorded block at R tokens per pass (default 3000, override
  `FACTORY_REVIEW_TOKENS`); N blocks → approximately N×R tokens of *review* spend
  avoided — printed as an **estimate** with its method, never as a bare "saved
  N tokens".

Historical output excerpt from the script factory, with two recorded blocks:

```
Facts (deterministic, 0 model tokens)
  Deterministic gates installed:  11  (run 'factory doctor' for armed/inert)
  Cost profile:                   economy
  Gate blocks recorded:           2
    - 2026-07-20T10:12Z  commit-message-lint     uncited 'verified' claim
    - 2026-07-20T11:03Z  direct-main-push-block  direct push to main

Estimate (labeled)
  Review-spend avoided:  ~6000 tokens
    = 2 block(s) x ~3000 tokens per LLM review pass ...

Not shown: a "tokens saved" headline. A real saved figure comes from an A/B eval.
```

Facts first, then one labeled estimate, and an explicit refusal of the vanity
number. The report reads as trustworthy because every line means what it says —
which, for a tool whose pitch is that it does not overclaim, is the version that
reinforces the product.

### The only honest source of a real "saved" number

A genuine savings figure has exactly one honest source: a measured A/B in the
eval harness — run the same task with the cost profile on and off and compare the
actual token counts. That is a real counterfactual because both runs actually
happened. Report the two runs, their settings and the
command that produced the comparison — not a per-session badge. Running every task twice to
show a savings number would itself double the spend, which is why this lives in
the eval, deliberately, and not in the session loop.

## Phased plan

- **Phase 0 — this document.** Make the levers explicit and name "prefer a hook
  over an LLM check" and "keep the prefix cache-friendly" as design values.
- **Phase 1 — the `economy` tier and profile. (Shipped.)** `factory init` prompts
  for `cost_profile` and records it with the economy tier in `factory.yaml`;
  the economy-eligible roles (`refactorer`, `wiki-maintainer`, opencode
  `small_model`) route to the economy tier under `economy` and collapse to the
  default under `standard`. The synchronizer writes the corresponding
  Claude and Codex settings. The implementer stays on the default tier.
- **Phase 1.5 — per-harness intelligent defaults. (Shipped.)** Each harness
  carries its own per-tier models (the matrix above) instead of one shared
  string, so Claude and Codex get real frontier/default/economy ladders out of
  the box when tiers are selected. The synchronizer reads them from
  `factory.yaml`. Blank model settings inherit the harness configuration.
- **Phase 2 — cache-friendly context and a budget audit.** Order the always-on
  context so the cacheable prefix is contiguous; measure the always-loaded
  footprint and move what can be lazy.
- **Phase 3 — measurement. (Shipped.)** `factory report` shows the facts the
  factory computes (gates enforced at 0 tokens, model routing, and gate blocks
  caught — logged by `scripts/lib/events.sh`), one labeled review-spend estimate,
  and a pointer to the harness for measured token spend. It never prints a
  "tokens saved" headline. A self-test fixture proves a firing logs an event and
  the report reads it back.
- **Phase 4 — eval-gated implementer downgrade.** Only after the eval shows a
  cheaper implementer still passes the gates, allow `economy` to route it to
  the economy tier.

## Risks and honesty

- **A cheaper model that fails gates costs more.** Retries and rework can wipe out
  the per-turn saving. This is why downgrades are eval-gated, not assumed.
- **Caching is harness-dependent.** The stable-prefix saving (L3) is only real if
  the harness caches it. The design makes the factory cache-*friendly*; it cannot
  force a harness to cache.
- **The factory structures work; it does not shrink a model's intrinsic token
  use.** It can route to a cheaper model and cut flailing, not make a given model
  cheaper per token.
- **Never downgrade the review path.** `reviewer` and `spec-writer` are
  low-volume and correctness-critical; keeping them frontier is cheap and is the
  thing that makes running a cheaper implementer safe. A profile that saves
  tokens by weakening the adversary is a false economy.

## What this plan will not do

- It will not relax a gate to save tokens. The gates are the safety margin that
  makes cheaper models viable; trading them away removes the reason a downgrade
  was safe.
- It will not trim governance context to the point the agent misbehaves. A cheap
  agent that has to be corrected twice is not cheap.
- It will not publish savings figures the eval has not produced. When there are
  numbers, they will come with the command that produced them.
- It will not print a per-session "you saved N tokens" headline. That is a
  counterfactual — it compares the run to a run that never happened — so it is a
  guess dressed as a measurement, the exact fabricated-metric failure this
  project has been burned by. The post-session report shows what the factory did
  (facts), what the work cost (measured), and one clearly-labeled estimate;
  a real savings figure comes only from a measured A/B in the eval.

## Bounded repair choices

[Loop engineering](LOOPS.md) defaults to manual checks without model calls.
Explicit bounded runs share budget admission for every implementer and reviewer
invocation. Loop progress detection prevents unchanged-source retries; selected
limits never lower reviewer quality or grant automatic merge permission.
