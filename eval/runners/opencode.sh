#!/bin/sh
# eval/runners/opencode.sh
# Drives opencode headlessly against a golden task. Shipped, not a template: the
# factory already treats opencode.json as canonical and routes this harness's
# model tiers, so it should not also ask you to work out the invocation.
#
# Usage (via the eval, which selects it by name):
#   ./scripts/golden-task-eval.sh --harness opencode
#
# Runner contract: $1 is a workdir containing task.md. Write the implementation
# into that directory. Exit status is ignored — verify.sh scores the result.
#
# Runs as the implementer role, so what is measured is the agent the factory
# actually routes work to — including the test-edit boundary it runs under.
# Evaluating some other configuration would be measuring a system nobody uses.
WORKDIR="$1"
[ -n "$WORKDIR" ] || { echo "opencode runner: no workdir given" >&2; exit 1; }
cd "$WORKDIR" || exit 1
command -v opencode >/dev/null 2>&1 || {
  echo "opencode runner: the 'opencode' CLI is not on PATH" >&2; exit 1; }

set -- run --agent implementer
[ -n "${FACTORY_EVAL_MODEL:-}" ] && set -- "$@" --model "$FACTORY_EVAL_MODEL"

opencode "$@" "$(cat task.md)" >opencode-runner.log 2>&1
exit 0
