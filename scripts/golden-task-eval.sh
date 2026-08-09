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
#   ./scripts/golden-task-eval.sh --timeout=600            # per-run cap (default 300s)
#
# Runner contract: `runner <workdir>` — reads <workdir>/task.md, writes an
# implementation into <workdir>. Exit status ignored; verify.sh scores. A real
# runner drives your harness (opencode/Claude/Codex) with your keys. See
# eval/README.md.

HARNESS="mock"
RUNNER="eval/runners/mock.sh"
RUNS=1
TIMEOUT=300
EVAL_DIR="eval/golden-tasks"
RESULTS_DIR="eval/results"
SAVE_BASELINE=false

for arg in "$@"; do
  case $arg in
    --harness=*)     HARNESS="${arg#*=}" ;;
    --runner=*)      RUNNER="${arg#*=}" ;;
    --runs=*)        RUNS="${arg#*=}" ;;
    --timeout=*)     TIMEOUT="${arg#*=}" ;;
    --save-baseline) SAVE_BASELINE=true ;;
  esac
done
case "$TIMEOUT" in ''|*[!0-9]*) TIMEOUT=300 ;; esac
[ "$TIMEOUT" -ge 1 ] || TIMEOUT=300
case "$RUNS" in ''|*[!0-9]*) RUNS=1 ;; esac
[ "$RUNS" -ge 1 ] || RUNS=1

mkdir -p "$RESULTS_DIR"
BASELINE_FILE="$RESULTS_DIR/${HARNESS}-baseline.json"
CURRENT_FILE="$RESULTS_DIR/${HARNESS}-current.json"

echo "golden-task-eval: harness=$HARNESS runner=$RUNNER runs=$RUNS"

# Tasks are directories under $EVAL_DIR containing task.md + verify.sh.
TASKS=$(find "$EVAL_DIR" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | sort || true)
if [ -z "$TASKS" ]; then
  echo "golden-task-eval: no tasks in $EVAL_DIR/"
  echo "  A task is a red spec plus an oracle — see eval/README.md."
  echo "  If eval/golden-tasks/ is missing entirely, 'factory upgrade' installs it."
  # No result file. An empty run measured nothing, and a results file that says
  # so is still a results file: it invites a later comparison against a baseline
  # of nothing, and it makes an unconfigured repo look instrumented.
  exit 0
fi

if [ ! -x "$RUNNER" ]; then
  echo "golden-task-eval: runner not executable: $RUNNER" >&2
  exit 1
fi
RUNNER_ABS="$(cd "$(dirname "$RUNNER")" && pwd)/$(basename "$RUNNER")"

TMP_RESULTS="$(mktemp)"
TMP_INPUTS="$(mktemp)"
trap 'rm -f "$TMP_RESULTS" "$TMP_INPUTS"' EXIT

# fingerprint <file>... — a checksum of the concatenated inputs. A score is only
# comparable to a baseline measured against the same inputs; when they differ the
# baseline is stale, not passing. Missing files contribute nothing.
fingerprint() { cat "$@" 2>/dev/null | cksum | awk '{print $1 "-" $2}'; }

# run_with_timeout <secs> <cmd>... — a wall-clock cap on one run.
# A headless agent can HANG rather than fail: a subagent that hits an "ask"
# permission queues a request nothing will service, and waits forever. Without a
# cap the eval wedges instead of scoring, so a hang is turned into a failed run.
# Implemented by hand because macOS ships no timeout(1).
run_with_timeout() {
  _rt_secs="$1"; shift
  # Job control gives the child its own process group, so a timeout can kill the
  # whole tree. Killing just the runner leaves its children orphaned — they keep
  # running and hold the inherited pipe open, which stalls whatever invoked us.
  set -m
  "$@" >/dev/null 2>&1 &
  _rt_pid=$!
  set +m
  _rt_waited=0
  while kill -0 "$_rt_pid" 2>/dev/null; do
    if [ "$_rt_waited" -ge "$_rt_secs" ]; then
      kill -9 -"$_rt_pid" 2>/dev/null || kill -9 "$_rt_pid" 2>/dev/null || true
      wait "$_rt_pid" 2>/dev/null || true
      return 124
    fi
    sleep 1
    _rt_waited=$((_rt_waited + 1))
  done
  wait "$_rt_pid" 2>/dev/null || true
}

# Inputs that affect every task: the runner (what drives the agent), the shared
# instructions, and the model tiers. Change any of them and the saved scores were
# measured against a different system.
# Each input is optional — a missing AGENTS.md or config file contributes nothing
# rather than failing the run (cat/grep exit non-zero under set -e).
#
# Both config files are read. Settings moved into factory.yaml in Decision 41,
# and reading only the old one would mean a model change no longer invalidated a
# baseline — the exact silent-staleness failure this fingerprint exists to catch.
{
  cat "$RUNNER_ABS" 2>/dev/null || true
  cat AGENTS.md 2>/dev/null || true
  grep -E '^(cost_profile|[a-z]+_(frontier|default|economy)_model):' factory.yaml 2>/dev/null || true
  grep -E '^(COST_PROFILE|[A-Z]+_(FRONTIER|DEFAULT|ECONOMY)_MODEL)=' factory.config 2>/dev/null || true
} > "$TMP_INPUTS"
INPUTS_FP="$(fingerprint "$TMP_INPUTS")"

for TASKDIR in $TASKS; do
  TASK_NAME="$(basename "$TASKDIR")"
  if [ ! -f "$TASKDIR/verify.sh" ]; then
    echo "  - $TASK_NAME: SKIP (no verify.sh oracle)"
    continue
  fi
  passes=0
  timeouts=0
  for _ in $(seq 1 "$RUNS"); do
    work="$(mktemp -d)"
    cp -R "$TASKDIR"/. "$work"/
    before="$(cksum "$work/verify.sh" | awk '{print $1, $2}')"
    rt=0
    run_with_timeout "$TIMEOUT" "$RUNNER_ABS" "$work" || rt=$?
    if [ "$rt" -eq 124 ]; then timeouts=$((timeouts + 1)); fi
    after="$(cksum "$work/verify.sh" 2>/dev/null | awk '{print $1, $2}')"
    # A run passes only if the oracle is untouched (no cheating) and it exits 0.
    if [ "$before" = "$after" ] && ( cd "$work" && sh verify.sh ) >/dev/null 2>&1; then
      passes=$((passes + 1))
    fi
    rm -rf "$work"
  done
  score="$(awk "BEGIN { printf \"%.2f\", $passes / $RUNS }")"
  # The task's own inputs: the spec and its oracle. If either changes, an old
  # score for this task is no longer a like-for-like comparison.
  task_fp="$(fingerprint "$TASKDIR/task.md" "$TASKDIR/verify.sh")"
  TIMEOUT_NOTE=""
  [ "$timeouts" -gt 0 ] && TIMEOUT_NOTE=" — $timeouts run(s) hit the ${TIMEOUT}s cap (a hung runner counts as failed)"
  echo "  - $TASK_NAME: $passes/$RUNS passed (score $score)$TIMEOUT_NOTE"
  printf '%s\t%s\t%s\t%s\t%s\n' "$TASK_NAME" "$passes" "$RUNS" "$score" "$task_fp" >> "$TMP_RESULTS"
done

python3 - "$HARNESS" "$RUNNER" "$RUNS" "$TMP_RESULTS" "$CURRENT_FILE" "$INPUTS_FP" <<'PY'
import json, sys
harness, runner, runs, tmp, out, inputs_fp = sys.argv[1:7]
tasks = []
with open(tmp) as f:
    for line in f:
        name, passes, r, score, fp = line.rstrip("\n").split("\t")
        tasks.append({"task": name, "passes": int(passes), "runs": int(r),
                      "score": float(score), "fingerprint": fp})
with open(out, "w") as fh:
    json.dump({"harness": harness, "runner": runner, "runs": int(runs),
               "inputs_fingerprint": inputs_fp, "tasks": tasks}, fh, indent=2)
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

# A saved score only means something against the inputs it was measured on. If
# those changed, the honest answer is "stale" — not "no regression", which would
# be claiming something this run cannot know.
stale = []
base_inputs = base.get("inputs_fingerprint")
legacy = base_inputs is None
if not legacy and base_inputs != cur.get("inputs_fingerprint"):
    stale.append(("all tasks", "the runner, AGENTS.md, or model tiers changed"))

b = {t["task"]: t for t in base.get("tasks", [])}
for t in cur.get("tasks", []):
    prev = b.get(t["task"])
    if not prev:
        continue  # a new task has nothing to compare against — not a regression
    pf, cf = prev.get("fingerprint"), t.get("fingerprint")
    if pf is not None and cf is not None and pf != cf:
        stale.append((t["task"], "its task.md or verify.sh oracle changed"))

if stale:
    print("golden-task-eval: BASELINE STALE — cannot claim 'no regression'")
    for name, reason in stale:
        print(f"  {name}: {reason}")
    print("The saved scores were measured against different inputs. Re-run with")
    print("--save-baseline to re-measure deliberately.")
    sys.exit(1)

regressions = [(t["task"], b[t["task"]]["score"], t["score"])
               for t in cur.get("tasks", [])
               if t["task"] in b and t["score"] < b[t["task"]]["score"] - 1e-9]
if regressions:
    print("golden-task-eval: REGRESSION DETECTED — a task's pass rate dropped")
    for name, was, now in regressions:
        print(f"  {name}: {was:.2f} -> {now:.2f}")
    print("If the change is intentional, re-run with --save-baseline to update.")
    sys.exit(1)

if legacy:
    print("golden-task-eval: baseline predates fingerprinting — staleness unchecked")
    print("  Re-run with --save-baseline to record what the scores were measured against.")
print("golden-task-eval: no regression from baseline")
PY
else
  echo "golden-task-eval: no baseline for '$HARNESS' — run with --save-baseline to create one"
fi

echo "golden-task-eval: done"
