#!/bin/bash
set -euo pipefail

# docs/DECISION_LOG.md:1681 — configuration stays in the shared parsed reader.
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=lib/config.sh
. "$SCRIPT_DIR/lib/config.sh"
# shellcheck source=lib/roles.sh
. "$SCRIPT_DIR/lib/roles.sh"

export FACTORY_BUDGET_ROOT
FACTORY_BUDGET_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
for entry in \
  'enabled false' 'max_attempts 1' 'max_session_runs 5' \
  'timeout_seconds 300' 'session_seconds 900' 'max_concurrent 1' \
  'estimated_usd' 'action stop'
do
  read -r key default <<< "$entry"
  value="$(factory_config_get "budget_$key" "${default:-}")"
  var="FACTORY_BUDGET_$(printf '%s' "$key" | tr '[:lower:]' '[:upper:]')"
  printf -v "$var" '%s' "$value"
  export "${var?}"
done

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
    factory_config_export
    tier="$(resolve_tier "${COST_PROFILE:-standard}" "$(role_tier "$budget_role")")"
    model_var="$(printf '%s_%s_MODEL' "$budget_harness" "$tier" | tr '[:lower:]' '[:upper:]')"
    FACTORY_BUDGET_MODEL="${!model_var:-}"
    ;;
esac
export FACTORY_BUDGET_MODEL
exec python3 "$SCRIPT_DIR/lib/budget.py" "$@"
