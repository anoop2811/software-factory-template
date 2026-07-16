#!/bin/bash
set -euo pipefail

# scripts/golden-task-eval.sh
# Runs the golden-task factory evals. These are NOT product tests — they are
# canonical coding stories run through the full agent loop to detect
# regressions when a model, prompt, or AGENTS.md changes.
#
# Usage:
#   ./scripts/golden-task-eval.sh                    # default harness
#   ./scripts/golden-task-eval.sh --harness=opencode # specific harness
#   ./scripts/golden-task-eval.sh --harness=claude   # specific harness
#
# Output: a score per golden task, saved to eval/results/<harness>-current.json.
# Diffs against eval/results/<harness>-baseline.json. Exits 1 if any score regressed.
# Does NOT overwrite the baseline — use --save-baseline to update it deliberately.

HARNESS="opencode"
EVAL_DIR="eval/golden-tasks"
RESULTS_DIR="eval/results"
SAVE_BASELINE=false

mkdir -p "$RESULTS_DIR"

# Parse args
for arg in "$@"; do
  case $arg in
    --harness=*) HARNESS="${arg#*=}" ;;
    --save-baseline) SAVE_BASELINE=true ;;
  esac
done

BASELINE_FILE="$RESULTS_DIR/${HARNESS}-baseline.json"
CURRENT_FILE="$RESULTS_DIR/${HARNESS}-current.json"

echo "golden-task-eval: harness=$HARNESS"
echo "golden-task-eval: looking for tasks in $EVAL_DIR/"

TASKS=$(find "$EVAL_DIR" -name '*.md' -not -name 'README.md' 2>/dev/null | sort || true)

if [ -z "$TASKS" ]; then
  echo "golden-task-eval: no tasks found (OK — pre-Phase-2)"
  echo "{\"harness\":\"$HARNESS\",\"tasks\":[],\"status\":\"no-tasks\"}" > "$CURRENT_FILE"
  exit 0
fi

# Run each task and score it
# Phase 2: these are stubs returning "pending". Phase 3 fills in real agent-loop execution.
CURRENT_RESULTS="{\"harness\":\"$HARNESS\",\"tasks\":["
FIRST=true

for TASK in $TASKS; do
  TASK_NAME=$(basename "$TASK" .md)
  echo "  - $TASK_NAME: pending (stub — real eval in Phase 3)"

  if [ "$FIRST" = true ]; then
    FIRST=false
  else
    CURRENT_RESULTS+=","
  fi
  CURRENT_RESULTS+="{\"task\":\"$TASK_NAME\",\"score\":\"pending\"}"
done

CURRENT_RESULTS+="]}"

echo "$CURRENT_RESULTS" | python3 -m json.tool > "$CURRENT_FILE" 2>/dev/null || echo "$CURRENT_RESULTS" > "$CURRENT_FILE"

# Save as baseline if requested
if [ "$SAVE_BASELINE" = true ]; then
  cp "$CURRENT_FILE" "$BASELINE_FILE"
  echo "golden-task-eval: baseline saved to $BASELINE_FILE"
  exit 0
fi

# Diff against baseline if it exists
if [ -f "$BASELINE_FILE" ]; then
  BASELINE_SCORES=$(python3 -c "
import json
with open('$BASELINE_FILE') as f:
    data = json.load(f)
for t in data.get('tasks', []):
    print(f\"{t['task']}={t['score']}\")
" 2>/dev/null || echo "")

  CURRENT_SCORES=$(python3 -c "
import json
with open('$CURRENT_FILE') as f:
    data = json.load(f)
for t in data.get('tasks', []):
    print(f\"{t['task']}={t['score']}\")
" 2>/dev/null || echo "")

  if [ "$BASELINE_SCORES" != "$CURRENT_SCORES" ]; then
    echo "golden-task-eval: REGRESSION DETECTED — scores changed from baseline"
    diff <(echo "$BASELINE_SCORES") <(echo "$CURRENT_SCORES") || true
    echo ""
    echo "If the change is intentional, run with --save-baseline to update."
    exit 1
  fi

  echo "golden-task-eval: no regression from baseline"
else
  echo "golden-task-eval: no baseline for harness=$HARNESS — run with --save-baseline to create one"
fi

echo "golden-task-eval: done"
