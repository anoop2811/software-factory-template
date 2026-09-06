#!/bin/bash
# Shared parsed settings and role model resolution. docs/LOOPS.md:44.
# Caller sources config.sh and roles.sh first.
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

factory_budget_model() {
  local harness="$1" role="$2" tier model_var
  tier="$(resolve_tier "${COST_PROFILE:-standard}" "$(role_tier "$role")")"
  model_var="$(printf '%s_%s_MODEL' "$harness" "$tier" | tr '[:lower:]' '[:upper:]')"
  printf '%s' "${!model_var:-}"
}
factory_config_export
