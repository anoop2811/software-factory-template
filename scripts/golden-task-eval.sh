#!/bin/bash
set -euo pipefail

# scripts/golden-task-eval.sh
# Runs the golden-task factory evals. Each task under eval/golden-tasks/<name>/ is
# a red acceptance spec (task.md) with an oracle (verify.sh, exit 0 = solved). A
# runner — a real agent, or the deterministic mock — must satisfy it. The score
# is the pass rate over N runs: verify.sh passes AND the runner did not tamper the
# oracle. Scores are diffed against a saved baseline; a drop is a regression.
#
# These are NOT product tests — they are canonical coding stories run through an
# agent loop to catch regressions when a model, prompt, or AGENTS.md changes.
#
# Usage:
#   ./scripts/golden-task-eval.sh                                   # mock runner, 1 run
#   ./scripts/golden-task-eval.sh --runner=eval/runners/opencode.sh --runs=5
#   ./scripts/golden-task-eval.sh --harness=claude --save-baseline
#
# Runner contract: `runner <workdir>` — reads <workdir>/task.md, writes an
# implementation into <workdir>. Exit status ignored; verify.sh scores. A real
# runner drives your harness (opencode/Claude/Codex) with your keys. See
# eval/README.md.

HARNESS="mock"
RUNNER="eval/runners/mock.sh"
RUNS=1
EVAL_DIR="eval/golden-tasks"
RESULTS_DIR="eval/results"
SAVE_BASELINE=false

for arg in "$@"; do
  case $arg in
    --harness=*)     HARNESS="${arg#*=}" ;;
    --runner=*)      RUNNER="${arg#*=}" ;;
    --runs=*)        RUNS="${arg#*=}" ;;
    --save-baseline) SAVE_BASELINE=true ;;
  esac
done
case "$RUNS" in ''|*[!0-9]*) RUNS=1 ;; esac
[ "$RUNS" -ge 1 ] || RUNS=1

mkdir -p "$RESULTS_DIR"
BASELINE_FILE="$RESULTS_DIR/${HARNESS}-baseline.json"
CURRENT_FILE="$RESULTS_DIR/${HARNESS}-current.json"

echo "golden-task-eval: harness=$HARNESS runner=$RUNNER runs=$RUNS"

# Tasks are directories under $EVAL_DIR containing task.md + verify.sh.
TASKS=$(find "$EVAL_DIR" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | sort || true)
if [ -z "$TASKS" ]; then
  echo "golden-task-eval: no tasks in $EVAL_DIR/ (OK — add task dirs to eval it)"
  printf '{"harness":"%s","tasks":[]}\n' "$HARNESS" > "$CURRENT_FILE"
  exit 0
fi

if [ ! -x "$RUNNER" ]; then
  echo "golden-task-eval: runner not executable: $RUNNER" >&2
  exit 1
fi
RUNNER_ABS="$(cd "$(dirname "$RUNNER")" && pwd)/$(basename "$RUNNER")"

TMP_RESULTS="$(mktemp)"
trap 'rm -f "$TMP_RESULTS"' EXIT

for TASKDIR in $TASKS; do
  TASK_NAME="$(basename "$TASKDIR")"
  if [ ! -f "$TASKDIR/verify.sh" ]; then
    echo "  - $TASK_NAME: SKIP (no verify.sh oracle)"
    continue
  fi
  passes=0
  for _ in $(seq 1 "$RUNS"); do
    work="$(mktemp -d)"
    cp -R "$TASKDIR"/. "$work"/
    before="$(cksum "$work/verify.sh" | awk '{print $1, $2}')"
    ( "$RUNNER_ABS" "$work" ) >/dev/null 2>&1 || true
    after="$(cksum "$work/verify.sh" 2>/dev/null | awk '{print $1, $2}')"
    # A run passes only if the oracle is untouched (no cheating) and it exits 0.
    if [ "$before" = "$after" ] && ( cd "$work" && sh verify.sh ) >/dev/null 2>&1; then
      passes=$((passes + 1))
    fi
    rm -rf "$work"
  done
  score="$(awk "BEGIN { printf \"%.2f\", $passes / $RUNS }")"
  echo "  - $TASK_NAME: $passes/$RUNS passed (score $score)"
  printf '%s\t%s\t%s\t%s\n' "$TASK_NAME" "$passes" "$RUNS" "$score" >> "$TMP_RESULTS"
done

python3 - "$HARNESS" "$RUNNER" "$RUNS" "$TMP_RESULTS" "$CURRENT_FILE" <<'PY'
import json, sys
harness, runner, runs, tmp, out = sys.argv[1:6]
tasks = []
with open(tmp) as f:
    for line in f:
        name, passes, r, score = line.rstrip("\n").split("\t")
        tasks.append({"task": name, "passes": int(passes), "runs": int(r), "score": float(score)})
with open(out, "w") as fh:
    json.dump({"harness": harness, "runner": runner, "runs": int(runs), "tasks": tasks}, fh, indent=2)
PY

if [ "$SAVE_BASELINE" = true ]; then
  cp "$CURRENT_FILE" "$BASELINE_FILE"
  echo "golden-task-eval: baseline saved to $BASELINE_FILE"
  exit 0
fi

if [ -f "$BASELINE_FILE" ]; then
  python3 - "$BASELINE_FILE" "$CURRENT_FILE" <<'PY'
import json, sys
base = json.load(open(sys.argv[1]))
cur = json.load(open(sys.argv[2]))
b = {t["task"]: t["score"] for t in base.get("tasks", [])}
regressions = [(t["task"], b[t["task"]], t["score"])
               for t in cur.get("tasks", [])
               if t["task"] in b and t["score"] < b[t["task"]] - 1e-9]
if regressions:
    print("golden-task-eval: REGRESSION DETECTED — a task's pass rate dropped")
    for name, was, now in regressions:
        print(f"  {name}: {was:.2f} -> {now:.2f}")
    print("If the change is intentional, re-run with --save-baseline to update.")
    sys.exit(1)
print("golden-task-eval: no regression from baseline")
PY
else
  echo "golden-task-eval: no baseline for '$HARNESS' — run with --save-baseline to create one"
fi

echo "golden-task-eval: done"
