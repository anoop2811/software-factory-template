# eval/

Two different things live under this name. One is real today; one is plumbing
waiting for your content. Being clear about which is which matters more to us
than looking finished.

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

## Golden tasks — plumbing ships, content is yours

`scripts/golden-task-eval.sh` is a regression harness for model and prompt
changes: put task descriptions in `eval/golden-tasks/`, run the script, and it
scores each task, compares against a saved baseline (`--save-baseline` to
update deliberately), and exits non-zero on regression.

Honest status: the baseline/diff plumbing works; the scoring is a stub. It
records that a task ran, not how well. Wiring real agent-loop execution and
scoring is on you — golden tasks are inherently project-specific, so shipping
ours would be worse than shipping none.

`eval/results/` holds baselines (committed) and current runs (ignored).
