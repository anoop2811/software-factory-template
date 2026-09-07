#!/bin/bash
# Candidate Bash adapter: configuration stays data, Go selects ordered effects.
# Source-time constant only; no runtime invocation. docs/adr/0056-go-configuration-export-plans.md:25.

FACTORY_CONFIG_KEYS="COST_PROFILE MODEL_PROVIDER \
OPENCODE_FRONTIER_MODEL OPENCODE_DEFAULT_MODEL OPENCODE_ECONOMY_MODEL \
CLAUDE_FRONTIER_MODEL CLAUDE_DEFAULT_MODEL CLAUDE_ECONOMY_MODEL \
CODEX_FRONTIER_MODEL CODEX_DEFAULT_MODEL CODEX_ECONOMY_MODEL \
REVIEW_LANE REVIEW_MODEL REVIEW_API_KEY_SECRET REVIEW_REASONING_EFFORT REVIEW_OPENROUTER_PROVIDER"

_factory_config_call() (
  case "${FACTORY_RUNTIME_BINARY-}" in
    /*) ;;
    *)
      printf '%s\n' 'factory bridge: FACTORY_RUNTIME_BINARY must name an executable absolute regular file' >&2
      return 2 ;;
  esac
  if [ ! -f "$FACTORY_RUNTIME_BINARY" ] || [ ! -x "$FACTORY_RUNTIME_BINARY" ]; then
    printf '%s\n' 'factory bridge: FACTORY_RUNTIME_BINARY must name an executable absolute regular file' >&2
    return 2
  fi
  /usr/bin/env "FACTORY_CONFIG=${FACTORY_CONFIG-}" FACTORY_BRIDGE_PROTOCOL=1 \
    "$FACTORY_RUNTIME_BINARY" "$@"
)

_factory_config_bad_plan() {
  printf '%s\n' 'factory bridge: invalid configuration plan' >&2
  return 2
}

# Arithmetic and reference attributes can interpret or redirect an assignment.
# Refuse them at the candidate boundary before applying any record.
# docs/adr/0056-go-configuration-export-plans.md:73.
_factory_config_scalar() {
  local _factory_config_declaration _factory_config_attributes
  if _factory_config_declaration="$(declare -p "$1" 2>/dev/null)"; then
    case "$_factory_config_declaration" in
      'declare -'*) ;;
      *) _factory_config_bad_plan; return ;;
    esac
    _factory_config_attributes="${_factory_config_declaration#declare -}"
    _factory_config_attributes="${_factory_config_attributes%% *}"
    case "$_factory_config_attributes" in
      *[!rx-]*)
        printf 'factory bridge: unsupported variable attributes for %s\n' "$1" >&2
        return 2 ;;
    esac
  fi
  return 0
}

# Walk twice: all records must validate before any assignment is attempted.
# Peel tabs literally; IFS tab splitting would discard value whitespace.
# docs/adr/0056-go-configuration-export-plans.md:42.
_factory_config_walk() {
  local _factory_config_record _factory_config_key _factory_config_value
  local _factory_config_state=header _factory_config_operation _factory_config_tab=$'\t'
  while IFS= read -r _factory_config_record; do
    case "$_factory_config_state" in
      header)
        [ "$_factory_config_record" = FACTORY_CONFIG_PLAN_V1 ] || { _factory_config_bad_plan; return; }
        _factory_config_state=records
        continue ;;
      end) _factory_config_bad_plan; return ;;
    esac
    if [ "$_factory_config_record" = END ]; then
      _factory_config_state=end
      continue
    fi
    case "$_factory_config_record" in
      set"$_factory_config_tab"*"$_factory_config_tab"*)
        _factory_config_operation='set'
        _factory_config_record="${_factory_config_record#*"$_factory_config_tab"}"
        _factory_config_key="${_factory_config_record%%"$_factory_config_tab"*}"
        _factory_config_value="${_factory_config_record#*"$_factory_config_tab"}" ;;
      export"$_factory_config_tab"*)
        _factory_config_operation='export'
        _factory_config_key="${_factory_config_record#*"$_factory_config_tab"}" ;;
      *) _factory_config_bad_plan; return ;;
    esac
    case "$_factory_config_key" in
      ''|*[!A-Z0-9_]*) _factory_config_bad_plan; return ;;
    esac
    case " $FACTORY_CONFIG_KEYS " in
      *" $_factory_config_key "*) ;;
      *) _factory_config_bad_plan; return ;;
    esac
    if [ "$2" = validate ]; then
      _factory_config_scalar "$_factory_config_key" || return
    fi
    if [ "$2" = apply ]; then
      if [ "$_factory_config_operation" = set ]; then
        printf -v "$_factory_config_key" '%s' "$_factory_config_value"
      fi
      export "${_factory_config_key?}"
    fi
  done <<< "$1"
  [ "$_factory_config_state" = end ] || { _factory_config_bad_plan; return; }
  return 0
}

_factory_config_plan() {
  local _factory_config_payload
  # The sentinel preserves trailing newlines during command substitution.
  # A failed child never reaches validation or parent-variable application.
  _factory_config_payload="$(_factory_config_call "$@" && printf '.')" || return
  _factory_config_payload="${_factory_config_payload%.}"
  case "$_factory_config_payload" in
    *$'\n') _factory_config_payload="${_factory_config_payload%$'\n'}" ;;
    *) _factory_config_bad_plan; return ;;
  esac
  _factory_config_walk "$_factory_config_payload" validate || return
  _factory_config_walk "$_factory_config_payload" apply
}

factory_config_export() {
  local _factory_config_caller_key
  set --
  for _factory_config_caller_key in $FACTORY_CONFIG_KEYS; do
    if [ -n "${!_factory_config_caller_key+set}" ]; then
      set -- "$@" "$_factory_config_caller_key"
    fi
  done
  _factory_config_plan config export "$@"
}

factory_config_load_legacy() {
  local _factory_config_legacy_file="$1" _factory_config_preserved="${2-}" _factory_config_caller_key
  set --
  for _factory_config_caller_key in $FACTORY_CONFIG_KEYS; do
    case " $_factory_config_preserved " in
      *" $_factory_config_caller_key "*) set -- "$@" "$_factory_config_caller_key" ;;
    esac
  done
  _factory_config_plan config legacy "$_factory_config_legacy_file" "$@"
}
