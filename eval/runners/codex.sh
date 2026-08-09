#!/bin/sh
# eval/runners/codex.sh
# Drives Codex headlessly against a golden task. Shipped, not a template: the
# factory already generates this harness's agent files and routes its model
# tiers, so it should not also ask you to work out the invocation.
#
# Usage (via the eval, which selects it by name):
#   ./scripts/golden-task-eval.sh --harness codex
#
# Runner contract: $1 is a workdir containing task.md. Write the implementation
# into that directory. Exit status is ignored — verify.sh scores the result.
#
# `workspace-write` is the narrowest sandbox that lets the task be solved: the
# agent may write in its own workdir and nowhere else. The alternative flag
# bypasses approvals AND the sandbox, which is a different bargain entirely —
# an eval is not a reason to hand over the machine.
WORKDIR="$1"
[ -n "$WORKDIR" ] || { echo "codex runner: no workdir given" >&2; exit 1; }
cd "$WORKDIR" || exit 1
command -v codex >/dev/null 2>&1 || {
  echo "codex runner: the 'codex' CLI is not on PATH" >&2; exit 1; }

set -- exec --sandbox workspace-write --skip-git-repo-check
[ -n "${FACTORY_EVAL_MODEL:-}" ] && set -- "$@" --model "$FACTORY_EVAL_MODEL"

codex "$@" "$(cat task.md)" >codex-runner.log 2>&1
exit 0
