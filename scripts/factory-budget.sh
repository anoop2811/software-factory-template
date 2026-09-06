#!/bin/bash
set -euo pipefail

# docs/DECISION_LOG.md:1681 — configuration stays in the shared parsed reader.
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=lib/config.sh
. "$SCRIPT_DIR/lib/config.sh"
# shellcheck source=lib/roles.sh
. "$SCRIPT_DIR/lib/roles.sh"

# shellcheck source=lib/budget-config.sh
. "$SCRIPT_DIR/lib/budget-config.sh"

# Python validates all arguments. This scan only chooses the shared model tier.
budget_harness=""
budget_role="implementer"
args=("$@")
for ((i=0; i<${#args[@]}; i++)); do
  case "${args[$i]}" in
    --harness) budget_harness="${args[$((i+1))]:-}" ;;
    --harness=*) budget_harness="${args[$i]#*=}" ;;
    --role) budget_role="${args[$((i+1))]:-}" ;;
    --role=*) budget_role="${args[$i]#*=}" ;;
  esac
done
FACTORY_BUDGET_MODEL=""
case "$budget_harness" in
  codex|claude|opencode)
    FACTORY_BUDGET_MODEL="$(factory_budget_model "$budget_harness" "$budget_role")"
    ;;
esac
export FACTORY_BUDGET_MODEL
exec python3 "$SCRIPT_DIR/lib/budget.py" "$@"
