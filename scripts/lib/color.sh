#!/bin/sh
# Factory native color helper: source-v1
# docs/adr/0137-go-source-shared-helpers.md:19.
# Inherited Bash metadata alone cannot select Bash-only syntax.
# shellcheck disable=SC3028
if command test -n "${BASH_VERSION-}" && command test -n "${BASH_SOURCE-}" &&
   command test "$(command type -t builtin 2>/dev/null)" = builtin; then
  # These features execute only in the Bash-capable branch.
  # shellcheck disable=SC3043,SC3045,SC3054,SC3003
  _factory_source_color_load() {
    builtin local _factory_source_root
    builtin test ! -L "${BASH_SOURCE[0]}" || {
      builtin printf '%s\n' 'factory color: source the physical library instead of a file symlink' >&2
      return 2
    }
    _factory_source_root=${BASH_SOURCE[0]%/*}
    builtin test "$_factory_source_root" != "${BASH_SOURCE[0]}" || _factory_source_root=.
    _factory_source_root="$(CDPATH='' builtin cd -P -- "$_factory_source_root/../.." && builtin pwd -P && builtin printf '.')" || return
    _factory_source_root=${_factory_source_root%$'\n.'}
    # shellcheck source=../../runtime/shell/helpers.sh
    builtin . "$_factory_source_root/runtime/shell/helpers.sh" || return
    # shellcheck source=../../runtime/shell/source.sh
    builtin . "$_factory_source_root/runtime/shell/source.sh"
  }
  _factory_source_color_load || return
  # shellcheck disable=SC3045
  builtin unset -f _factory_source_color_load
else
  # The canonical plan interpreter must be sourced into this shell.
  # docs/adr/0137-go-source-shared-helpers.md:45.
  _factory_posix_color_root() (
    if [ "${FACTORY_SOURCE_ROOT+x}" = x ]; then
      case "$FACTORY_SOURCE_ROOT" in
        /*) _factory_posix_root=$FACTORY_SOURCE_ROOT ;;
        *) command printf '%s\n' 'factory color: FACTORY_SOURCE_ROOT must name an absolute source checkout' >&2; return 2 ;;
      esac
    else
      _factory_posix_root="$(command git rev-parse --show-toplevel 2>/dev/null && command printf '.')" || {
        command printf '%s\n' 'factory color: supply FACTORY_SOURCE_ROOT outside a Git repository' >&2
        return 2
      }
      case "$_factory_posix_root" in
        *'
.') ;;
        *) command printf '%s\n' 'factory color: supply FACTORY_SOURCE_ROOT outside a Git repository' >&2; return 2 ;;
      esac
      _factory_posix_root=${_factory_posix_root%??}
    fi
    CDPATH='' command cd -P "$_factory_posix_root" && command pwd -P
  )
  _factory_posix_color_load() {
    # Supported POSIX source shells provide local, as the canonical bridge does.
    # shellcheck disable=SC3043
    command local _factory_posix_root
    _factory_posix_root="$(_factory_posix_color_root && command printf '.')" || return
    _factory_posix_root=${_factory_posix_root%??}
    [ -f "$_factory_posix_root/runtime/shell/helpers.sh" ] &&
      [ -r "$_factory_posix_root/runtime/shell/helpers.sh" ] || {
        command printf '%s\n' 'factory color: the selected checkout has no canonical helper bridge' >&2
        return 2
      }
    # shellcheck source=../../runtime/shell/helpers.sh
    command . "$_factory_posix_root/runtime/shell/helpers.sh"
  }
  _factory_posix_color_load || return
  unset -f _factory_posix_color_load
  _factory_helper_call() (
    if [ "${FACTORY_RUNTIME_BINARY+x}" = x ]; then
      case "$FACTORY_RUNTIME_BINARY" in
        /*) _factory_posix_runtime=$FACTORY_RUNTIME_BINARY ;;
        *) command printf '%s\n' 'factory bridge: FACTORY_RUNTIME_BINARY must name an executable absolute regular file' >&2; return 2 ;;
      esac
    else
      # Loading the canonical definitions may replace other public helpers.
      # Their caller-Git ownership must not change with source order.
      # docs/adr/0137-go-source-shared-helpers.md:36.
      case "${1-}" in
        palette|action-box)
          _factory_posix_runtime="$(_factory_posix_color_root && command printf '.')" || return ;;
        *)
          _factory_posix_runtime="$(command git rev-parse --show-toplevel 2>/dev/null && command printf '.')" || {
            command printf '%s\n' 'factory helper: supply an explicit runtime outside a Git repository' >&2
            return 2
          }
          case "$_factory_posix_runtime" in
            *'
.') ;;
            *) command printf '%s\n' 'factory helper: supply an explicit runtime outside a Git repository' >&2; return 2 ;;
          esac ;;
      esac
      _factory_posix_runtime=${_factory_posix_runtime%??}/factory
    fi
    [ -f "$_factory_posix_runtime" ] && [ -x "$_factory_posix_runtime" ] || {
      command printf '%s\n' 'factory color: selected owner must be an executable regular file' >&2
      return 2
    }
    exec /usr/bin/env "FACTORY_CONFIG=${FACTORY_CONFIG-}" "NO_COLOR=${NO_COLOR-}" \
      "FACTORY_EVENT_LOG=${FACTORY_EVENT_LOG-}" "FACTORY_EVENT_MAX_LINES=${FACTORY_EVENT_MAX_LINES-}" \
      FACTORY_BRIDGE_PROTOCOL=1 "$_factory_posix_runtime" helper "$@"
  )
fi
factory_palette
