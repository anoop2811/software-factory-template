#!/bin/sh
# Factory native role reader: source-v1
# docs/adr/0136-go-source-configuration-entrypoints.md:58.
# macOS sh exposes Bash source paths; true POSIX callers use the lazy branch.
# Metadata can be inherited by another shell; lookup must identify a builtin,
# without executing a same-named PATH program merely to select the branch.
# shellcheck disable=SC3028 # Optional metadata is read safely before capability selection.
if command test -n "${BASH_VERSION-}" && command test -n "${BASH_SOURCE-}" &&
   command test "$(command type -t builtin 2>/dev/null)" = builtin; then
  # These features execute only in the Bash-capable branch.
  # shellcheck disable=SC3043,SC3045,SC3054,SC3003
  _factory_source_roles_load() {
    builtin local _factory_source_root
    builtin test ! -L "${BASH_SOURCE[0]}" || {
      builtin printf '%s\n' 'factory role: source the physical library instead of a file symlink' >&2
      return 2
    }
    _factory_source_root=${BASH_SOURCE[0]%/*}
    builtin test "$_factory_source_root" != "${BASH_SOURCE[0]}" || _factory_source_root=.
    _factory_source_root="$(CDPATH='' builtin cd -P -- "$_factory_source_root/../.." && builtin pwd -P && builtin printf '.')" || return
    _factory_source_root=${_factory_source_root%$'\n.'}
    # shellcheck source=../../runtime/shell/readers.sh
    builtin . "$_factory_source_root/runtime/shell/readers.sh" || return
    # shellcheck source=../../runtime/shell/source.sh
    builtin . "$_factory_source_root/runtime/shell/source.sh"
  }
  _factory_source_roles_load || return
  # shellcheck disable=SC3045 # This cleanup is still inside the Bash branch.
  builtin unset -f _factory_source_roles_load
else
  _factory_posix_role_call() (
    if [ "${FACTORY_RUNTIME_BINARY+x}" = x ]; then
      case "$FACTORY_RUNTIME_BINARY" in
        /*) ;;
        *) printf '%s\n' 'factory bridge: FACTORY_RUNTIME_BINARY must name an executable absolute regular file' >&2; return 2 ;;
      esac
      [ -f "$FACTORY_RUNTIME_BINARY" ] && [ -x "$FACTORY_RUNTIME_BINARY" ] || {
        printf '%s\n' 'factory bridge: FACTORY_RUNTIME_BINARY must name an executable absolute regular file' >&2
        return 2
      }
      exec /usr/bin/env "FACTORY_CONFIG=${FACTORY_CONFIG-}" FACTORY_BRIDGE_PROTOCOL=1 \
        "$FACTORY_RUNTIME_BINARY" role "$@"
    fi
    _factory_posix_runtime="$(git rev-parse --show-toplevel 2>/dev/null && printf '.')" || {
      printf '%s\n' 'factory role: supply an explicit runtime outside a Git repository' >&2
      return 2
    }
    case "$_factory_posix_runtime" in
      *'
.') ;;
      *) printf '%s\n' 'factory role: supply an explicit runtime outside a Git repository' >&2; return 2 ;;
    esac
    _factory_posix_runtime=${_factory_posix_runtime%??}/factory
    [ -f "$_factory_posix_runtime" ] && [ -x "$_factory_posix_runtime" ] || {
      printf '%s\n' 'factory role: the caller repository has no executable factory owner' >&2
      return 2
    }
    exec /usr/bin/env "FACTORY_CONFIG=${FACTORY_CONFIG-}" FACTORY_BRIDGE_PROTOCOL=1 \
      "$_factory_posix_runtime" role "$@"
  )
  role_tier() { _factory_posix_role_call tier "$1"; }
  resolve_tier() { _factory_posix_role_call resolve "$1" "$2"; }
fi
