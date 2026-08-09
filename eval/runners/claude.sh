#!/bin/sh
# eval/runners/claude.sh
# Drives Claude Code headlessly against a golden task. Shipped, not a template:
# the factory already generates this harness's agent files and routes its model
# tiers, so it should not also ask you to work out the invocation.
#
# Usage (via the eval, which selects it by name):
#   ./scripts/golden-task-eval.sh --harness claude
#
# Runner contract: $1 is a workdir containing task.md. Write the implementation
# into that directory. Exit status is ignored — verify.sh scores the result.
#
# Permissions are the trap here. A headless run has nobody to approve anything,
# and an "ask" permission does not fail cleanly: the primary session auto-rejects
# it, so the work is blocked and the task fails as though the model were
# incapable. acceptEdits grants file edits without granting shell — enough for a
# task whose acceptance is a file, and short of handing over the machine. A task
# that needs to run commands should raise this deliberately, not discover it.
WORKDIR="$1"
[ -n "$WORKDIR" ] || { echo "claude runner: no workdir given" >&2; exit 1; }
cd "$WORKDIR" || exit 1
command -v claude >/dev/null 2>&1 || {
  echo "claude runner: the 'claude' CLI is not on PATH" >&2; exit 1; }

# The model comes from the tier the factory routed for this harness, when one is
# set; otherwise Claude Code uses its own configured default. The eval exports
# FACTORY_EVAL_MODEL for exactly this.
set -- -p --permission-mode acceptEdits
[ -n "${FACTORY_EVAL_MODEL:-}" ] && set -- "$@" --model "$FACTORY_EVAL_MODEL"

# The task text is the prompt. Nothing is added to it: the point is to measure
# the agent against the spec an engineer would have handed it.
claude "$@" "$(cat task.md)" >claude-runner.log 2>&1
exit 0
