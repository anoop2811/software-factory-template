#!/bin/bash
# scripts/lib/config.sh
# Reader for factory.yaml — the template's runtime configuration (Decision 2).
#
# factory.yaml is a deliberately constrained format so this parser stays tiny:
#   - flat `key: value` pairs, one per line, no nesting
#   - lists are space-separated values on one line
#   - values may be double-quoted; a trailing ` # comment` is stripped
# Anything more expressive belongs in a hook, not in configuration.
#
# Usage (from a hook, after sourcing this file):
#   value="$(factory_config_get test_file_patterns)"
#   value="$(factory_config_get check_command 'make check')"   # with default
#
# FACTORY_CONFIG overrides the config path (used by the break/fix self-tests).

factory_config_file() {
  if [ -n "${FACTORY_CONFIG:-}" ]; then
    printf '%s' "$FACTORY_CONFIG"
    return
  fi
  local root
  root="$(git rev-parse --show-toplevel 2>/dev/null || echo .)"
  printf '%s/factory.yaml' "$root"
}

factory_config_get() {
  local key="$1"
  local default="${2:-}"
  local file value
  file="$(factory_config_file)"
  if [ ! -f "$file" ]; then
    printf '%s' "$default"
    return
  fi
  value="$(sed -n "s/^${key}:[[:space:]]*//p" "$file" | head -n 1)"
  # Unwrap in the order the format defines: a quoted value ends at its closing
  # quote and anything after it is a comment; an unquoted value ends at ` #`.
  # Quotes are checked first so that a hash *inside* them stays part of the
  # value — a model string or a prompt fragment may legitimately contain one.
  case "$value" in
    \"*)
      value="${value#\"}"
      value="${value%%\"*}"
      ;;
    *)
      value="$(printf '%s' "$value" | sed 's/[[:space:]]#.*$//; s/[[:space:]]*$//')"
      ;;
  esac
  if [ -z "$value" ]; then
    printf '%s' "$default"
    return
  fi
  printf '%s' "$value"
}

# factory_config_export: load the settings scripts need as shell variables.
#
# These used to live in factory.config, which was *sourced* — configuration an
# adopter edits, executed by every script that read it. They are now ordinary
# factory.yaml keys, parsed like everything else (Decision 41). One file, read
# and never run.
#
# Each variable is only set when the YAML defines it, so anything already in the
# environment still wins — that is what lets a caller override a model tier for
# one run. Repositories that predate the move keep working: factory.config is
# sourced first when it exists, and the YAML overlays whatever it defines. That
# fallback is deliberately quiet here; `factory doctor` is where it is reported,
# so a deprecation cannot live forever by going unnoticed.
# factory_config_load_legacy <file>: export the settings a pre-YAML factory.config
# defines, by PARSING it — never by sourcing it.
#
# Sourcing was the original implementation and it undid the property the move to
# factory.yaml exists to provide. factory.config lives in the repository, so it
# can arrive from a branch, a patch or a pull request; sourcing it executed
# whatever it contained, with the privileges of whatever called this — including
# CI. `factory migrate-config` already parsed the same file for exactly this
# reason ("parsing it beats sourcing it"), so this only makes the library agree
# with the tool that replaces it.
#
# Same grammar as that parser: KEY=value, optional single or double quotes, a
# trailing ` #` comment on unquoted values only, and keys restricted to shell
# identifiers. Only the fixed key list below is exported, so an unexpected name
# in the file cannot set an arbitrary variable.
# The settings the factory recognises. Both the YAML and the legacy reader are
# restricted to this list, so neither file can set a variable the factory did
# not ask for.
FACTORY_CONFIG_KEYS="COST_PROFILE MODEL_PROVIDER \
OPENCODE_FRONTIER_MODEL OPENCODE_DEFAULT_MODEL OPENCODE_ECONOMY_MODEL \
CLAUDE_FRONTIER_MODEL CLAUDE_DEFAULT_MODEL CLAUDE_ECONOMY_MODEL \
CODEX_FRONTIER_MODEL CODEX_DEFAULT_MODEL CODEX_ECONOMY_MODEL \
REVIEW_LANE REVIEW_MODEL REVIEW_API_KEY_SECRET"

factory_config_load_legacy() {
  local file="$1" line key value upper
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in
      ''|'#'*) continue ;;
      *=*) : ;;
      *) continue ;;
    esac
    key="${line%%=*}"
    value="${line#*=}"
    # Strip `export ` and surrounding whitespace, so `export FOO=bar` is read too.
    key="${key#export }"
    key="$(printf '%s' "$key" | tr -d '[:space:]')"
    case "$key" in
      ''|*[!A-Za-z0-9_]*) continue ;;
    esac
    case "$value" in
      \"*) value="${value#\"}"; value="${value%%\"*}" ;;
      \'*) value="${value#\'}"; value="${value%%\'*}" ;;
      *) value="$(printf '%s' "$value" | sed 's/[[:space:]]#.*$//; s/^[[:space:]]*//; s/[[:space:]]*$//')" ;;
    esac
    upper="$(printf '%s' "$key" | tr '[:lower:]' '[:upper:]')"
    case " $FACTORY_CONFIG_KEYS " in
      *" $upper "*) : ;;
      *) continue ;;
    esac
    # Assign indirectly without re-parsing the value, as the YAML path does.
    eval "$upper=\$value"
    export "${upper?}"
  done < "$file"
}

factory_config_export() {
  local file legacy root
  file="$(factory_config_file)"
  root="$(dirname "$file")"
  legacy="$root/factory.config"

  # Precedence has three sources, and they are not interchangeable:
  #   caller environment  >  factory.yaml  >  legacy factory.config
  # So the caller's variables have to be identified BEFORE the legacy file is
  # loaded — once it exports, "already set" can no longer tell a deliberate
  # override apart from the legacy value, and treating them alike would let the
  # legacy file beat the YAML, which is the opposite of the documented order.
  local caller_set="" k
  for k in $FACTORY_CONFIG_KEYS; do
    if [ -n "${!k+set}" ]; then
      caller_set="$caller_set $k"
    fi
  done

  if [ -f "$legacy" ]; then
    factory_config_load_legacy "$legacy"
  fi

  local key var value
  for key in \
    cost_profile model_provider \
    opencode_frontier_model opencode_default_model opencode_economy_model \
    claude_frontier_model claude_default_model claude_economy_model \
    codex_frontier_model codex_default_model codex_economy_model \
    review_lane review_model review_api_key_secret
  do
    value="$(factory_config_get "$key")"
    [ -n "$value" ] || continue
    var="$(printf '%s' "$key" | tr '[:lower:]' '[:upper:]')"
    # A value the caller put in the environment wins, which is what the comment
    # above this function promises — "anything already in the environment still
    # wins ... that is what lets a caller override a model tier for one run". The
    # assignment below used to overwrite it whenever the YAML had anything, so
    # `MODEL_PROVIDER=anthropic ./script` silently used the file's value. Only
    # variables recorded before the legacy load count, so the YAML still wins
    # over factory.config.
    case " $caller_set " in
      *" $var "*)
        export "${var?}"
        continue ;;
    esac
    # `eval "$var=\$value"` assigns indirectly. The backslash matters: it defers
    # $value to assignment time, where it is not re-parsed, so a model string
    # containing spaces or shell metacharacters lands verbatim rather than being
    # executed. The name is safe by construction — it comes from the fixed list
    # above, never from the file.
    eval "$var=\$value"
    # ${var?} rather than $var: exporting by computed name is intended here, and
    # the brace form is how that intent is stated (shellcheck SC2163).
    export "${var?}"
  done
}

# factory_config_set <key> <value>: record a setting in factory.yaml.
#
# Replaces the key in place when it exists, appends it otherwise, so the file
# keeps its shape and its comments. Values are written quoted, which the reader
# strips — a model string with a slash or a space survives the round trip.
#
# factory_config_has <key>: true when the key is present at all, including when
# its value is empty. Presence is meaningful on its own: an opt-in recorded as
# "off" is an answer that was given, not an answer that is missing, which is what
# lets the upgrade ask once and never nag.
factory_config_set() {
  local key="$1" value="$2" file
  file="$(factory_config_file)"
  if [ ! -f "$file" ]; then
    echo "factory config: no $(basename "$file") here — run factory init first." >&2
    return 1
  fi
  if grep -q "^${key}:" "$file"; then
    # A literal replacement, not a regex one: model strings contain slashes, so
    # the substitution needs a delimiter they cannot be mistaken for.
    sed -i.factory-bak "s|^${key}:.*|${key}: \"${value}\"|" "$file" &&
      rm -f "$file.factory-bak"
  else
    printf '%s: "%s"\n' "$key" "$value" >> "$file"
  fi
}

factory_config_has() {
  local file
  file="$(factory_config_file)"
  [ -f "$file" ] || return 1
  grep -q "^${1}:" "$file"
}
