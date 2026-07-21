# Cost and tokens

**Status: Phase 1 shipped; later phases are intent.** The opt-in `economy` cost
profile is live — `factory-init` offers it, and it routes the low-stakes roles
across opencode, Claude, and Codex (see the phased plan below for what has
landed and what has not). The default is unchanged: cost control is a profile
you turn on, not a change to how the factory works out of the box. Sections
describing later phases (context budget, measurement, the eval-gated implementer
downgrade) remain proposals, and say so.

**In short**

- There are two cost surfaces: the tokens the factory *itself* adds (governance
  context, review passes) and the tokens the *work* costs (the actual
  implementation). The second usually dwarfs the first.
- The largest lever already half-exists. The factory routes roles to a
  `DEFAULT_MODEL` and a `FRONTIER_MODEL`. A cost profile extends that to a
  cheaper third tier for high-volume mechanical roles.
- A deterministic gate costs zero model tokens. Every check that stays a shell
  hook instead of an LLM reviewer is a check that never bills.
- Cost is opt-in. The default profile favours simplicity; the cost-optimized
  profile trades a little configuration for lower spend.
- We do not publish savings numbers we have not measured. Where this document
  estimates, it says "expected", not "reduces by X%".

## The two cost surfaces

**1. The cost of using the factory.** The overhead the factory imposes on top of
the raw task: governance context loaded into the prompt (`AGENTS.md`,
`FACTORY_RULES.md`, role prompts, the Verification Contract), plus the extra
turns discipline adds — a spec pass, a test pass, a reviewer pass. This is real
but bounded, and most of it is a stable prefix that caches well (see L3).

**2. The cost of the work done with the factory.** The tokens the actual
implementation burns — exploration, generation, rework. This is where the money
is, and it is mostly a function of which model does the work and how much it
flails. The factory reduces flailing (tight specs, fast deterministic feedback)
and can route the high-volume work to a cheaper model (L2).

A cost plan has to help both, and be honest that surface 2 is the larger prize.

## Where the factory spends tokens today

- **Always-loaded context.** The instruction files are read every turn. They are
  stable across turns, which matters for caching, but they are not free on the
  first turn or on any cache miss.
- **Per-role model routing already exists, two tiers.** `spec-writer` and
  `reviewer` run on `FRONTIER_MODEL`; `implementer`, `refactorer`, and
  `wiki-maintainer` run on `DEFAULT_MODEL`. Both model strings are chosen at
  `factory-init`, recorded in `factory.config`, and substituted into the harness
  configs (`opencode.json` and the agent frontmatter) — the role→model mapping
  lives there, not in `factory.yaml`, which holds the runtime enforcement values.
- **`small_model` now follows the economy tier.** The opencode lightweight-task
  model (titles, summaries) was pointed at `DEFAULT_MODEL`; under the `economy`
  profile it routes to the cheaper `ECONOMY_MODEL` instead. (This lever is
  opencode-specific — Claude and Codex have no equivalent per-project
  small-model knob, but they still get the per-role economy routing below.)
- **Gates are shell, so they are free.** The self-test, `factory doctor`, and
  every hook cost zero model tokens. This is the design already paying off:
  enforcement that would otherwise be an LLM review pass is a shell exit code.

## The levers

### L1 — Enforcement in shell, not the model

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
| `implementer` | high | medium (tests are the net) | default, or economy under the cost profile |
| `refactorer` | high | low (behaviour-preserving) | economy |
| `wiki-maintainer` | medium | low | economy |

The insight: once the spec and tests exist, the implementer's job is bounded and
checkable — a cheaper model that passes the gates is as good as an expensive one
that passes the gates. The correctness-critical roles are low-volume, so keeping
them on a frontier model is cheap. This is where a three-tier profile earns its
keep.

### L3 — A cache-friendly stable prefix

The factory's instruction files do not change turn to turn, which is the ideal
shape for prompt caching. If the harness caches the stable prefix, the governance
overhead (surface 1) becomes nearly free after the first turn. The design value:
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

A `COST_PROFILE`, chosen at `factory-init` and recorded in `factory.config`
alongside the models, defaulting to `standard`:

- **`standard`** — the economy-eligible roles collapse to each harness's default
  model, so there is no third tier and nothing to think about.
- **`economy`** — turns on a third, cheaper tier and routes the high-volume,
  low-stakes roles to it: `refactorer`, `wiki-maintainer`, the opencode
  `small_model`, and — once the eval proves it (see below) — `implementer`.
  `spec-writer` and `reviewer` stay frontier; the profile never downgrades the
  roles whose job is to catch mistakes.

### Per-harness models

The three harnesses have different native model namespaces — opencode can route
any provider through OpenRouter, Claude Code calls Anthropic ids, Codex calls
OpenAI ids — so each carries its own per-tier models, shipped as intelligent
defaults and overridable in `factory.config`:

| Tier | opencode (OpenRouter) | Codex | Claude |
|---|---|---|---|
| frontier | `openrouter/z-ai/glm-5.2` | `gpt-5.6-sol` | `claude-opus-4-8` |
| default | `openrouter/z-ai/glm-5.2` | `gpt-5.6-terra` | `claude-sonnet-4-6` |
| economy | `openrouter/qwen/qwen3-coder` | `gpt-5.6-luna` | `claude-haiku-4-5` |

```sh
# factory.config (excerpt)
COST_PROFILE="economy"
OPENCODE_FRONTIER_MODEL="openrouter/z-ai/glm-5.2"
OPENCODE_DEFAULT_MODEL="openrouter/z-ai/glm-5.2"
OPENCODE_ECONOMY_MODEL="openrouter/qwen/qwen3-coder"
CLAUDE_FRONTIER_MODEL="claude-opus-4-8"
CLAUDE_DEFAULT_MODEL="claude-sonnet-4-6"
CLAUDE_ECONOMY_MODEL="claude-haiku-4-5"
CODEX_FRONTIER_MODEL="gpt-5.6-sol"
CODEX_DEFAULT_MODEL="gpt-5.6-terra"
CODEX_ECONOMY_MODEL="gpt-5.6-luna"
```

The routing reaches all three harnesses through `make sync-harnesses`. A role's
tier is a property of the *role*, not the model string (opencode's frontier and
default can share one model), so `scripts/lib/roles.sh` maps each role to its
tier — `spec-writer`/`reviewer` → frontier, `refactorer`/`wiki-maintainer` →
economy, everything else → default. `sync-opencode` writes opencode's models
into `opencode.json`; `sync-claude`/`sync-codex` write each Claude/Codex
subagent's model. All three read that tier's model from `factory.config`, and a
blank value (or the template repo, which has no `factory.config`) falls back to
`inherit` rather than breaking. It all lives in `factory.config`, not
`factory.yaml`, and touches no runtime gate.

### Changing it later

Reconfiguring is one edit and one command — no re-init:

```sh
# change a model, or flip COST_PROFILE between standard and economy, in factory.config
$ make sync-harnesses
```

`factory.config` is the single source of truth for every harness's models,
including opencode. The economy→default collapse is applied at *sync* time (by
`resolve_tier` reading `COST_PROFILE`), so flipping the profile and re-syncing
re-routes opencode, Claude, and Codex together — you are never editing three
places or re-running init to change a tier. `factory-init` runs this same sync at
the end, so a fresh repo is already wired.

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

You cannot reduce what you cannot see. The shell factory does not meter tokens —
the harness does — but the factory can make that metering actionable: running
each role as its own session gives per-role cost visibility, so an adopter can
see whether the implementer or the reviewer is the real spend and tune the
profile accordingly. A later `factory` report that summarizes per-role model
assignment (not token counts, which live in the harness) is a possible addition,
not a commitment.

### A post-session report

A post-session report shows what the factory actually did for you this session.
It states three things in three registers and keeps them visibly separate, so
each number means exactly what it says:

- **Facts the factory computes itself (no counterfactual).** "This session: N
  deterministic gates fired, 0 model tokens. K fail-fast catches (a gate stopped
  a bad path before it generated downstream work)." A hook that exits non-zero
  genuinely cost no model tokens; this is what *happened*, not what was avoided.
- **Actual spend from the harness (measured, not saved).** If the harness exposes
  per-role token counts, show them: "implementer X on economy, reviewer Y on
  frontier." This is real metering and it is what lets an adopter tune the
  profile — no comparison to a phantom run.
- **At most one clearly-labeled estimate.** The only defensible "avoided" figure
  is narrow and anchored: each deterministic gate stands in for one LLM review
  pass; a review pass over this diff is roughly R tokens; N gates fired, so
  approximately N×R tokens of *review* spend were avoided — **estimate**, stated
  with its baseline assumption (that you would otherwise enforce these rules with
  a model). It is printed as an estimate with a pointer to this method, never as
  a bare "saved N tokens".

An illustrative report:

```
Factory this session: 6 deterministic checks enforced in shell (0 model tokens).
  1 fail-fast catch (bad commit blocked before push).
  Model routing: implementer=economy, reviewer=frontier.
  Review spend avoided (estimate = N gates x ~R tokens per review pass): ~N*R tokens.
  Method: docs/COST_AND_TOKENS.md.
```

Facts first, then one labeled estimate. The report reads as trustworthy because
every line means what it says — which, for a tool whose pitch is that it does not
overclaim, is the version that reinforces the product.

### The only honest source of a real "saved" number

A genuine savings figure has exactly one honest source: a measured A/B in the
eval harness — run the same task with the cost profile on and off and compare the
actual token counts. That is a real counterfactual because both runs actually
happened. It is an occasional eval artifact ("measured 2026-07-19, task X:
economy profile used 12% fewer tokens, gates still green"), reported with the
command that produced it — not a per-session badge. Running every task twice to
show a savings number would itself double the spend, which is why this lives in
the eval, deliberately, and not in the session loop.

## Phased plan

- **Phase 0 — this document.** Make the levers explicit and name "prefer a hook
  over an LLM check" and "keep the prefix cache-friendly" as design values.
- **Phase 1 — the `economy` tier and profile. (Shipped.)** `factory-init` prompts
  for `COST_PROFILE` and records it with `ECONOMY_MODEL` in `factory.config`;
  the economy-eligible roles (`refactorer`, `wiki-maintainer`, opencode
  `small_model`) route to the economy tier under `economy` and collapse to the
  default under `standard`. Routing crosses to Claude and Codex via the sync
  scripts. `implementer` stays on the default model until Phase 4. A break/fix
  fixture in the self-test proves Codex emits a per-agent model for a native id
  and omits it for a slug or placeholder.
- **Phase 1.5 — per-harness intelligent defaults. (Shipped.)** Each harness
  carries its own per-tier models (the matrix above) instead of one shared
  string, so Claude and Codex get real frontier/default/economy ladders out of
  the box, not just opencode. `scripts/lib/roles.sh` maps role → tier and the
  sync scripts read each tier's model from `factory.config`. Self-test fixtures
  prove per-tier routing on both Codex and Claude, and inherit when unset.
- **Phase 2 — cache-friendly context and a budget audit.** Order the always-on
  context so the cacheable prefix is contiguous; measure the always-loaded
  footprint and move what can be lazy.
- **Phase 3 — measurement.** Surface per-role model assignment so adopters can
  reason about where spend goes.
- **Phase 4 — eval-gated implementer downgrade.** Only after the eval shows a
  cheaper implementer still passes the gates, allow `economy` to route it to
  `ECONOMY_MODEL`.

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
