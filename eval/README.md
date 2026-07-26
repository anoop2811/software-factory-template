# eval/

Two evals live under this name. One proves the factory's wiring; the other scores
agents against tasks. Both are real — the second ships with a deterministic mock
runner and a reference task so it self-tests, but real scoring needs your tasks
and a runner for your harness. Being clear about which is which matters more to
us than looking finished.

## Structural harness eval — real, runs in CI

`make eval` runs `scripts/harness-structural-eval.sh` once per harness
(opencode, Claude Code, Codex). For each one it proves two things:

1. the generated adapter actually delegates to the shared deny script
   (asserted against the adapter files, not assumed), and
2. the deny script blocks an implementer edit and allows a spec-writer edit
   when fed that harness's documented payload shape.

It uses its own temporary config, so it passes or fails on wiring — not on
whatever your `factory.yaml` happens to arm. What it deliberately does not
claim: live in-harness behavior. That's a separate, manual observation.

## Golden tasks — real scoring; runners and tasks are yours

`scripts/golden-task-eval.sh` runs each task under `eval/golden-tasks/<name>/`
through a **runner** and **scores it for real**: the pass rate over N runs, where
a run passes only if the task's oracle (`verify.sh`) exits 0 **and** the runner
did not tamper with it. Scores are diffed against a saved baseline
(`--save-baseline` to update deliberately); a **drop** in any task's pass rate is
a regression and exits non-zero.

```sh
./scripts/golden-task-eval.sh                                  # mock runner, 1 run
./scripts/golden-task-eval.sh --runner=eval/runners/opencode.sh --runs=5
./scripts/golden-task-eval.sh --harness=claude --save-baseline
```

### A task

A directory `eval/golden-tasks/<name>/` with:

- `task.md` — the spec the runner must satisfy. A real task is a **red acceptance
  spec** (Ginkgo, pytest, JUnit) the implementer must make pass without editing
  the tests — the same loop the factory enforces.
- `verify.sh` — the oracle. Exit 0 = solved. Language-agnostic: for a Go task it
  runs `go test`; the shipped `reference-answer` task keeps it in pure shell so
  the harness self-tests anywhere. The eval fails a run if the runner changes
  this file (you cannot cheat the oracle).

### A runner

`runner <workdir>` — reads `<workdir>/task.md` and writes an implementation into
`<workdir>`. Its exit status is ignored; `verify.sh` scores the result. The
shipped `eval/runners/mock.sh` calls **no model** — it exercises the scorer both
ways so the harness is provable in CI without credentials (the self-test uses
it). A **real** runner drives your harness (opencode / Claude Code / Codex) with
your keys and budget, so it is opt-in and yours to wire — golden tasks and live
model runs are inherently project-specific.

**Headless permissions bite first.** A non-interactive run has nobody to approve
anything, and `ask` permissions do not fail cleanly: the primary session
auto-*rejects* them, so the task fails as though the model were incapable — while
a subagent can *hang*, its request joining a queue nothing will service. Grant
what the task needs as explicit `allow` in your harness config before evaluating,
or you are scoring your permission settings rather than the agent. Every run is
capped (`--timeout`, default 300s) so a hang scores 0 instead of wedging the
suite; a run that hits the cap usually means permissions, not model quality.

### A baseline can go stale

A saved score only means something against the inputs it was measured on. So each
result records a fingerprint of those inputs — per task, its `task.md` and
`verify.sh` oracle; globally, the runner, `AGENTS.md`, and the model tiers from
`factory.config`. When a fingerprint differs, the eval reports **stale** and exits
non-zero with the reason:

```
golden-task-eval: BASELINE STALE — cannot claim 'no regression'
  reference-answer: its task.md or verify.sh oracle changed
```

This matters because the failure is silent otherwise. Edit an oracle and the pass
rate can stay 1.00 while measuring something entirely different — the old code
would have printed "no regression" and been *wrong*. Stale is not a regression and
not a pass; it means this run cannot know, which is the Verification Contract
applied to time (`factory doctor` classifies gates the same way). Re-run with
`--save-baseline` to re-measure deliberately.

A baseline saved before fingerprinting still compares, with a warning that
staleness is unchecked — re-baseline once to arm it.

Why this shape: the scorer is deterministic and free to run; the expensive,
non-deterministic part (a live agent) is a pluggable runner behind a one-line
contract. That is what lets the *factory itself* stay break/fix-proven while the
*agent-quality* measurement runs where the credentials and tasks live. It is also
the foundation for eval-gated model choices (docs/COST_AND_TOKENS.md, Phase 4):
a role's tier drops only when the eval shows the cheaper model still passes.

`eval/results/` holds baselines (committed) and current runs (ignored);
`mock-*.json` are self-test artifacts and are ignored too.
