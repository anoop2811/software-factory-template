# Workflow: eval-fanout

> Score a model or prompt change across tasks, without one agent holding it all.

Maps onto scripts/golden-task-eval.sh: fan out runs, score each against its oracle,
aggregate the pass rates. The scoring and aggregation are deterministic edges.

## run
- role: implementer
- kind: fanout
- over: each golden task x model x N runs

## score
- role: code
- kind: verify
- of: run — each task's verify.sh oracle; a run counts only if it passes untampered

## aggregate
- role: code
- kind: edge
- note: pass rate per task, diffed against the baseline — plain code, no agent
