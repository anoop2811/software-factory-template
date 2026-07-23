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

Why this shape: the scorer is deterministic and free to run; the expensive,
non-deterministic part (a live agent) is a pluggable runner behind a one-line
contract. That is what lets the *factory itself* stay break/fix-proven while the
*agent-quality* measurement runs where the credentials and tasks live. It is also
the foundation for eval-gated model choices (docs/COST_AND_TOKENS.md, Phase 4):
a role's tier drops only when the eval shows the cheaper model still passes.

`eval/results/` holds baselines (committed) and current runs (ignored);
`mock-*.json` are self-test artifacts and are ignored too.
