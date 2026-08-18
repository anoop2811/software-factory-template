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

# Both spellings, and an error on anything unrecognised. The = form alone meant
# `--harness claude` was silently ignored: the run used the mock, reported
# "harness=mock", and an adopter reading a 1.00 would believe they had measured
# their agent. A flag that is quietly dropped is worse than one that is rejected.
RUNNER_EXPLICIT=""
# A flag whose value is missing must say so. Under `set -u` the bare shift left
# the script dying with no message and exit 1, which reads as a broken eval
# rather than a typo — the same class as the silently-ignored flag this parser
# was rewritten to fix.
need_value() {
  [ "$#" -ge 2 ] && [ -n "$2" ] && return 0
  echo "golden-task-eval: $1 needs a value" >&2
  exit 2
}
while [ "$#" -gt 0 ]; do
  case "$1" in
    --harness=*)     HARNESS="${1#*=}" ;;
    --harness)       need_value "$@"; HARNESS="$2"; shift ;;
    --runner=*)      RUNNER="${1#*=}"; RUNNER_EXPLICIT=1 ;;
    --runner)        need_value "$@"; RUNNER="$2"; RUNNER_EXPLICIT=1; shift ;;
    --runs=*)        RUNS="${1#*=}" ;;
    --runs)          need_value "$@"; RUNS="$2"; shift ;;
    --timeout=*)     TIMEOUT="${1#*=}" ;;
    --timeout)       need_value "$@"; TIMEOUT="$2"; shift ;;
    --save-baseline) SAVE_BASELINE=true ;;
    -h|--help)
      echo "usage: golden-task-eval.sh [--harness NAME] [--runner PATH] [--runs N] [--timeout S] [--save-baseline]"
      echo "  --harness  mock (default), claude, codex, opencode, or your own name"
      echo "             a shipped runner is selected automatically by name"
      exit 0 ;;
    *) echo "golden-task-eval: unknown argument '$1'" >&2; exit 2 ;;
  esac
  shift
done

# A named harness selects its shipped runner. The factory already configures
# these three — it generates their agent files and routes their model tiers — so
# it should not also ask you to work out the invocation. An explicit --runner
# still wins, for a harness the factory does not ship.
if [ -z "$RUNNER_EXPLICIT" ] && [ -f "eval/runners/$HARNESS.sh" ]; then
  RUNNER="eval/runners/$HARNESS.sh"
elif [ -z "$RUNNER_EXPLICIT" ] && [ "$HARNESS" != "mock" ]; then
  # Naming a harness the factory does not ship, with no --runner, used to fall
  # back to the mock runner. The mock solves every task, so the run reported a
  # perfect score for a harness that was never invoked — a measurement that looks
  # like evidence and is not. Refuse instead: the caller has to say what to run.
  echo "golden-task-eval: no shipped runner for harness '$HARNESS'." >&2
  # find, not `ls` on a glob: with eval/runners absent the glob goes unmatched and
  # `ls` fails, which under `set -o pipefail` makes this substitution non-zero and
  # leaves the reader an empty list where the reason was "the directory is gone".
  # Say which of the two it is.
  SHIPPED="$(find eval/runners -maxdepth 1 -type f -name '*.sh' 2>/dev/null |
    sed 's|.*/||; s|\.sh$||' | sort | tr '\n' ' ' || true)"
  if [ -n "${SHIPPED// /}" ]; then
    echo "  Shipped: $SHIPPED" >&2
  elif [ -d eval/runners ]; then
    echo "  Shipped: none — eval/runners is empty." >&2
  else
    echo "  Shipped: none — there is no eval/runners directory in this repo." >&2
  fi
  echo "  Pass --runner <script> for your own harness, or --harness mock to use the mock deliberately." >&2
  echo "  Not falling back to the mock: it solves every task, so the score would be meaningless." >&2
  exit 2
fi
case "$TIMEOUT" in ''|*[!0-9]*) TIMEOUT=300 ;; esac
[ "$TIMEOUT" -ge 1 ] || TIMEOUT=300
case "$RUNS" in ''|*[!0-9]*) RUNS=1 ;; esac
[ "$RUNS" -ge 1 ] || RUNS=1

mkdir -p "$RESULTS_DIR"
BASELINE_FILE="$RESULTS_DIR/${HARNESS}-baseline.json"
CURRENT_FILE="$RESULTS_DIR/${HARNESS}-current.json"

# Frontier tier for the harness under test, straight from factory.yaml via the
# same resolver the sync scripts use. Empty means "inherit", which is a valid
# answer: the runner then uses the CLI's own configuration.
# Resolved in this shell, so $HARNESS is only ever DATA. Interpolating it into a
# `bash -c` script made the value executable: `--harness 'x$(touch FILE)x'` ran
# the command, which a review caught and a fixture confirmed. A config lookup
# must never be able to run what it is looking up.
EVAL_MODEL=""
if [ -f scripts/lib/config.sh ]; then
  # shellcheck source=lib/config.sh
  . scripts/lib/config.sh
  EVAL_MODEL="$(factory_config_get "${HARNESS}_frontier_model" 2>/dev/null || true)"
fi
echo "golden-task-eval: harness=$HARNESS runner=$RUNNER runs=$RUNS model=${EVAL_MODEL:-inherit}"

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
    # The configured tier is passed through, so a run measures the model the
    # factory routes for this harness rather than whatever the CLI defaults to.
    # Without it a score says nothing about the configuration it was meant to
    # evaluate. Runners that ignore the variable are unaffected.
    rt=0
    FACTORY_EVAL_MODEL="$EVAL_MODEL" \
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
  if [ "$HARNESS" = "mock" ]; then
    echo "  Note: this is the mock runner. It calls no model and always scores 1.00 —"
    echo "  it proves the scorer, not your agents. To measure agents, point --runner"
    echo "  at a real harness (see eval/runners/example-harness.sh) and --harness at"
    echo "  its name, so the baseline records which system was scored."
  fi
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

# A task in the baseline with no current result is never compared, so deleting a
# failing task used to slip past the gate — the easiest possible way to make a
# regression disappear. Treat it as stale: the run is no longer like-for-like.
current_names = {t["task"] for t in cur.get("tasks", [])}
for name in b:
    if name not in current_names:
        stale.append((name, "in the baseline but absent from this run (task removed?)"))

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
