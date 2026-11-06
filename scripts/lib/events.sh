#!/bin/sh
# Factory native events helper: source-v1
# docs/adr/0137-go-source-shared-helpers.md:19.
# Inherited Bash metadata alone cannot select Bash-only syntax.
# shellcheck disable=SC3028
if command test -n "${BASH_VERSION-}" && command test -n "${BASH_SOURCE-}" &&
   command test "$(command type -t builtin 2>/dev/null)" = builtin; then
  # These features execute only in the Bash-capable branch.
  # shellcheck disable=SC3043,SC3045,SC3054,SC3003
  _factory_source_events_load() {
    builtin local _factory_source_root
    builtin test ! -L "${BASH_SOURCE[0]}" || {
      builtin printf '%s\n' 'factory events: source the physical library instead of a file symlink' >&2
      return 2
    }
    _factory_source_root=${BASH_SOURCE[0]%/*}
    builtin test "$_factory_source_root" != "${BASH_SOURCE[0]}" || _factory_source_root=.
    _factory_source_root="$(CDPATH='' builtin cd -P -- "$_factory_source_root/../.." && builtin pwd -P && builtin printf '.')" || return
    _factory_source_root=${_factory_source_root%$'\n.'}
    # Telemetry must not change a gate decision in an incomplete checkout.
    # docs/adr/0137-go-source-shared-helpers.md:61.
    if ! builtin test -f "$_factory_source_root/runtime/shell/helpers.sh" ||
       ! builtin test -r "$_factory_source_root/runtime/shell/helpers.sh" ||
       ! builtin test -f "$_factory_source_root/runtime/shell/source.sh" ||
       ! builtin test -r "$_factory_source_root/runtime/shell/source.sh"; then
      factory_log_event() { :; }
      return 0
    fi
    # shellcheck source=../../runtime/shell/helpers.sh
    builtin . "$_factory_source_root/runtime/shell/helpers.sh" || return
    # shellcheck source=../../runtime/shell/source.sh
    builtin . "$_factory_source_root/runtime/shell/source.sh"
  }
  _factory_source_events_load || return
  # shellcheck disable=SC3045
  builtin unset -f _factory_source_events_load
else
  # POSIX sourcing is inert; ownership is selected only at invocation.
  # docs/adr/0137-go-source-shared-helpers.md:36.
  _factory_posix_events_call() (
    if [ "${FACTORY_RUNTIME_BINARY+x}" = x ]; then
      case "$FACTORY_RUNTIME_BINARY" in
        /*) ;;
        *) command printf '%s\n' 'factory bridge: FACTORY_RUNTIME_BINARY must name an executable absolute regular file' >&2; return 2 ;;
      esac
      _factory_posix_runtime=$FACTORY_RUNTIME_BINARY
    else
      _factory_posix_runtime="$(command git rev-parse --show-toplevel 2>/dev/null && command printf '.')" || {
        command printf '%s\n' 'factory events: supply an explicit runtime outside a Git repository' >&2
        return 2
      }
      case "$_factory_posix_runtime" in
        *'
.') ;;
        *) command printf '%s\n' 'factory events: supply an explicit runtime outside a Git repository' >&2; return 2 ;;
      esac
      _factory_posix_runtime=${_factory_posix_runtime%??}/factory
    fi
    [ -f "$_factory_posix_runtime" ] && [ -x "$_factory_posix_runtime" ] || {
      command printf '%s\n' 'factory events: selected owner must be an executable regular file' >&2
      return 2
    }
    exec /usr/bin/env "FACTORY_CONFIG=${FACTORY_CONFIG-}" "NO_COLOR=${NO_COLOR-}" \
      "FACTORY_EVENT_LOG=${FACTORY_EVENT_LOG-}" "FACTORY_EVENT_MAX_LINES=${FACTORY_EVENT_MAX_LINES-}" \
      FACTORY_BRIDGE_PROTOCOL=1 "$_factory_posix_runtime" helper "$@"
  )
  factory_log_event() { _factory_posix_events_call event "${1:-gate}" "${2:-blocked}" >/dev/null 2>&1 || :; }
fi
