#!/bin/bash
# Native Go source adapter; docs/adr/0108-default-go-cutover.md:86.
# Inert sourceable helper definitions; only explicit calls have effects.
# docs/adr/0099-native-factory-orchestration.md:140.
# shellcheck disable=SC2034 # Palette variables are intentionally consumed by callers.

_factory_helper_call() (
  if [ -n "${FACTORY_RUNTIME_BINARY-}" ]; then
    case "$FACTORY_RUNTIME_BINARY" in /*) ;; *) return 2 ;; esac
    [ -f "$FACTORY_RUNTIME_BINARY" ] && [ -x "$FACTORY_RUNTIME_BINARY" ] || return 2
    _factory_helper_runtime=$FACTORY_RUNTIME_BINARY
  elif [ -n "${BASH_VERSION-}" ]; then
    _factory_helper_root=${BASH_SOURCE[0]%/*}
    [ "$_factory_helper_root" != "${BASH_SOURCE[0]}" ] || _factory_helper_root=.
    _factory_helper_root="$(CDPATH='' cd -- "$_factory_helper_root/../.." && pwd -P && printf '.')" || return
    _factory_helper_root=${_factory_helper_root%??}
    _factory_helper_runtime=$_factory_helper_root/factory
  else
    _factory_helper_root="$(git rev-parse --show-toplevel 2>/dev/null && printf '.')" || return
    _factory_helper_root=${_factory_helper_root%??}
    _factory_helper_runtime=$_factory_helper_root/factory
  fi
  /usr/bin/env "FACTORY_CONFIG=${FACTORY_CONFIG-}" "NO_COLOR=${NO_COLOR-}" \
    "FACTORY_EVENT_LOG=${FACTORY_EVENT_LOG-}" "FACTORY_EVENT_MAX_LINES=${FACTORY_EVENT_MAX_LINES-}" \
    FACTORY_BRIDGE_PROTOCOL=1 "$_factory_helper_runtime" helper "$@"
)

factory_log_event() {
  _factory_helper_call event "${1:-gate}" "${2:-blocked}" >/dev/null 2>&1 || :
}

factory_now() {
  _factory_helper_call now 2>/dev/null || printf '0'
}

factory_duration() {
  if [ -n "${2-}" ]; then
    _factory_helper_call duration "${1:-0}" "$2" 2>/dev/null || printf 'unknown'
  else
    _factory_helper_call duration "${1:-0}" 2>/dev/null || printf 'unknown'
  fi
}

action_box() {
  _factory_helper_call action-box "${C_RESET-}" "${C_BOLD-}" "${C_YELLOW-}" "$@"
}

hookspath_status() {
  _factory_helper_call hookspath "$1"
}

_factory_helper_bad_plan() {
  printf '%s\n' 'factory bridge: invalid helper plan' >&2
  return 2
}

_factory_helper_config_key() {
  case "$1" in
    COST_PROFILE|MODEL_PROVIDER|OPENCODE_FRONTIER_MODEL|OPENCODE_DEFAULT_MODEL|OPENCODE_ECONOMY_MODEL|\
    CLAUDE_FRONTIER_MODEL|CLAUDE_DEFAULT_MODEL|CLAUDE_ECONOMY_MODEL|\
    CODEX_FRONTIER_MODEL|CODEX_DEFAULT_MODEL|CODEX_ECONOMY_MODEL|\
    REVIEW_LANE|REVIEW_MODEL|REVIEW_API_KEY_SECRET|REVIEW_REASONING_EFFORT|REVIEW_OPENROUTER_PROVIDER|REVIEW_MAX_TOKENS) return 0 ;;
    *) return 1 ;;
  esac
}

_factory_helper_budget_key() {
  case "$1" in
    FACTORY_BUDGET_ROOT|FACTORY_BUDGET_ENABLED|FACTORY_BUDGET_MAX_ATTEMPTS|FACTORY_BUDGET_MAX_SESSION_RUNS|\
    FACTORY_BUDGET_TIMEOUT_SECONDS|FACTORY_BUDGET_SESSION_SECONDS|FACTORY_BUDGET_MAX_CONCURRENT|\
    FACTORY_BUDGET_ESTIMATED_USD|FACTORY_BUDGET_ACTION) return 0 ;;
    *) _factory_helper_config_key "$1" ;;
  esac
}

# Reject readonly, arithmetic, reference and array attributes before any effect.
# docs/adr/0099-native-factory-orchestration.md:143.
_factory_helper_scalar() {
  local _factory_helper_declaration _factory_helper_attributes
  if command -v declare >/dev/null 2>&1 && _factory_helper_declaration="$(declare -p "$1" 2>/dev/null)"; then
    case "$_factory_helper_declaration" in
      'declare -'*) ;;
      *) _factory_helper_bad_plan; return ;;
    esac
    _factory_helper_attributes="${_factory_helper_declaration#declare -}"
    _factory_helper_attributes="${_factory_helper_attributes%% *}"
    case "$_factory_helper_attributes" in
      *[!x-]*) _factory_helper_bad_plan; return ;;
    esac
  fi
  return 0
}

# Parse twice using literal delimiters. No plan record is shell code.
_factory_helper_walk() {
  local _factory_helper_record _factory_helper_key _factory_helper_value _factory_helper_operation
  local _factory_helper_state=header _factory_helper_tab='\t'
  _factory_helper_tab="$(printf '\t')"
  while IFS= read -r _factory_helper_record; do
    case "$_factory_helper_state" in
      header)
        [ "$_factory_helper_record" = "$2" ] || { _factory_helper_bad_plan; return; }
        _factory_helper_state=records
        continue ;;
      end) _factory_helper_bad_plan; return ;;
    esac
    if [ "$_factory_helper_record" = END ]; then
      _factory_helper_state=end
      continue
    fi
    case "$_factory_helper_record" in
      set"$_factory_helper_tab"*"$_factory_helper_tab"*)
        _factory_helper_operation='set'
        _factory_helper_record="${_factory_helper_record#*"$_factory_helper_tab"}"
        _factory_helper_key="${_factory_helper_record%%"$_factory_helper_tab"*}"
        _factory_helper_value="${_factory_helper_record#*"$_factory_helper_tab"}" ;;
      export"$_factory_helper_tab"*)
        _factory_helper_operation='export'
        _factory_helper_key="${_factory_helper_record#*"$_factory_helper_tab"}" ;;
      *) _factory_helper_bad_plan; return ;;
    esac
    if [ "$2" = FACTORY_HELPER_PALETTE_V1 ]; then
      [ "$_factory_helper_operation" = set ] || { _factory_helper_bad_plan; return; }
      case "$_factory_helper_key" in
        C_RESET|C_BOLD|C_DIM|C_YELLOW|C_GREEN|C_RED) ;;
        *) _factory_helper_bad_plan; return ;;
      esac
    else
      _factory_helper_budget_key "$_factory_helper_key" || { _factory_helper_bad_plan; return; }
    fi
    if [ "$3" = validate ]; then
      _factory_helper_scalar "$_factory_helper_key" || return
    elif [ "$2" = FACTORY_HELPER_PALETTE_V1 ]; then
      case "$_factory_helper_key" in
        C_RESET) C_RESET=$_factory_helper_value ;;
        C_BOLD) C_BOLD=$_factory_helper_value ;;
        C_DIM) C_DIM=$_factory_helper_value ;;
        C_YELLOW) C_YELLOW=$_factory_helper_value ;;
        C_GREEN) C_GREEN=$_factory_helper_value ;;
        C_RED) C_RED=$_factory_helper_value ;;
      esac
    else
      if [ "$_factory_helper_operation" = set ]; then
        printf -v "$_factory_helper_key" '%s' "$_factory_helper_value" || return
      fi
      export "${_factory_helper_key?}" || return
    fi
  done <<FACTORY_HELPER_RECORDS
$1
FACTORY_HELPER_RECORDS
  [ "$_factory_helper_state" = end ] || { _factory_helper_bad_plan; return; }
  return 0
}

_factory_helper_plan() {
  local _factory_helper_header="$1" _factory_helper_payload
  local LC_ALL=C
  shift
  # A sentinel retains the exact trailing newline through command substitution.
  _factory_helper_payload="$(_factory_helper_call "$@" && printf '.')" || return
  _factory_helper_payload="${_factory_helper_payload%.}"
  [ "${#_factory_helper_payload}" -le 1048576 ] || { _factory_helper_bad_plan; return; }
  case "$_factory_helper_payload" in
    *'
') _factory_helper_payload="${_factory_helper_payload%'
'}" ;;
    *) _factory_helper_bad_plan; return ;;
  esac
  _factory_helper_walk "$_factory_helper_payload" "$_factory_helper_header" validate || return
  _factory_helper_walk "$_factory_helper_payload" "$_factory_helper_header" apply
}

factory_palette() {
  if [ -t 1 ]; then
    _factory_helper_plan FACTORY_HELPER_PALETTE_V1 palette 1
  else
    _factory_helper_plan FACTORY_HELPER_PALETTE_V1 palette 0
  fi
}

factory_budget_config() {
  local _factory_helper_caller_key
  set --
  for _factory_helper_caller_key in COST_PROFILE MODEL_PROVIDER \
    OPENCODE_FRONTIER_MODEL OPENCODE_DEFAULT_MODEL OPENCODE_ECONOMY_MODEL \
    CLAUDE_FRONTIER_MODEL CLAUDE_DEFAULT_MODEL CLAUDE_ECONOMY_MODEL \
    CODEX_FRONTIER_MODEL CODEX_DEFAULT_MODEL CODEX_ECONOMY_MODEL \
    REVIEW_LANE REVIEW_MODEL REVIEW_API_KEY_SECRET REVIEW_REASONING_EFFORT REVIEW_OPENROUTER_PROVIDER REVIEW_MAX_TOKENS; do
    if declare -p "$_factory_helper_caller_key" >/dev/null 2>&1; then
      set -- "$@" "$_factory_helper_caller_key"
    fi
  done
  _factory_helper_plan FACTORY_BUDGET_PLAN_V1 budget-plan "$@"
}

factory_budget_model() {
  case "$1" in
    codex) _factory_helper_call budget-model "$1" "$2" "${COST_PROFILE:-standard}" "${CODEX_FRONTIER_MODEL-}" "${CODEX_DEFAULT_MODEL-}" "${CODEX_ECONOMY_MODEL-}" ;;
    claude) _factory_helper_call budget-model "$1" "$2" "${COST_PROFILE:-standard}" "${CLAUDE_FRONTIER_MODEL-}" "${CLAUDE_DEFAULT_MODEL-}" "${CLAUDE_ECONOMY_MODEL-}" ;;
    opencode) _factory_helper_call budget-model "$1" "$2" "${COST_PROFILE:-standard}" "${OPENCODE_FRONTIER_MODEL-}" "${OPENCODE_DEFAULT_MODEL-}" "${OPENCODE_ECONOMY_MODEL-}" ;;
    *) return 0 ;;
  esac
}

factory_budget_config
