#!/bin/sh
# Candidate sourceable readers, with no work at source time.
# Explicit local runtime selection only. docs/DECISION_LOG.md:1970.

_factory_reader_call() (
  case "${FACTORY_RUNTIME_BINARY-}" in
    /*) ;;
    *)
      printf '%s\n' 'factory bridge: FACTORY_RUNTIME_BINARY must name an executable absolute regular file' >&2
      return 2
      ;;
  esac
  if [ ! -f "$FACTORY_RUNTIME_BINARY" ] || [ ! -x "$FACTORY_RUNTIME_BINARY" ]; then
    printf '%s\n' 'factory bridge: FACTORY_RUNTIME_BINARY must name an executable absolute regular file' >&2
    return 2
  fi
  # Child-only values must not assign to a caller's readonly shell variables.
  # The candidate Linux/macOS runtime requires /usr/bin/env.
  /usr/bin/env "FACTORY_CONFIG=${FACTORY_CONFIG-}" FACTORY_BRIDGE_PROTOCOL=1 \
    "$FACTORY_RUNTIME_BINARY" "$@"
)

factory_config_file() {
  _factory_reader_call config file
}

factory_config_get() {
  _factory_reader_call config get "$1" "${2-}"
}

factory_config_has() {
  _factory_reader_call config has "$1"
}

role_tier() {
  _factory_reader_call role tier "$1"
}

resolve_tier() {
  _factory_reader_call role resolve "$1" "$2"
}

# Expansion belongs to the host shell; grouping belongs to Go.
# docs/adr/0062-go-local-hook-normalization.md:32.
factory_local_hooks() (
  # Positional parameters cannot inherit integer or nameref variable attributes.
  # Frame status separately so a failed read never becomes hook data.
  set -- "$(if factory_config_get local_hooks; then
    printf '\n0'
  else
    printf '\n%d' "$?"
  fi)"
  case "${1##*
}" in
    0) ;;
    *) return "${1##*
}" ;;
  esac
  # Recapture only the value, retaining command substitution's LF trimming.
  set -- "$(printf '%s' "${1%
*}")"
  case "$1" in
    *,*)
      set -- "$1" "${IFS+x}" "${IFS-}"
      IFS=','
      # shellcheck disable=SC2086 # Preserve baseline splitting and pathname expansion.
      set -- "$2" "$3" $1
      if [ "$1" = x ]; then
        IFS=$2
      else
        unset IFS
      fi
      shift 2
      _factory_reader_call config hooks commas "$@"
      ;;
    *)
      # shellcheck disable=SC2086 # Preserve the caller's IFS and glob options.
      set -- $1
      _factory_reader_call config hooks words "$@"
      ;;
  esac
)
