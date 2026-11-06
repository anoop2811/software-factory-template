#!/bin/bash
# Factory native check adapter: source-v1
# docs/adr/0138-go-source-check-orchestration.md:15.
#
# Run manually: ./scripts/pre-push-check.sh
# Copy this source adapter as a Git pre-push hook:
#   cp scripts/pre-push-check.sh .git/hooks/pre-push
#   chmod +x .git/hooks/pre-push
# Installed-copy setup is separate: docs/adr/0138-go-source-check-orchestration.md:80.

# Bash can locate a readable PATH script while leaving $0 as a bare filename.
# docs/adr/0138-go-source-check-orchestration.md:117.
_factory_source_declaration="$(builtin declare -p BASH_SOURCE 2>/dev/null)"
case "${_factory_source_declaration%% BASH_SOURCE=*}" in
  'declare -'*a*) ;;
  *)
    builtin printf '%s\n' 'factory check: native Bash source metadata is required' >&2
    exit 2
    ;;
esac
_factory_source_path=${BASH_SOURCE[0]}
builtin test ! -L "$_factory_source_path" || {
  builtin printf '%s\n' 'factory check: invoke the physical source adapter instead of a file symlink' >&2
  exit 2
}

case "${_factory_source_path##*/}" in
  pre-push)
    # Copied hooks select the caller project, never a missing direct-owner fallback.
    # docs/adr/0138-go-source-check-orchestration.md:32.
    if builtin test -n "${REPO_ROOT-}"; then
      _factory_root=$REPO_ROOT
    elif _factory_root="$(command git rev-parse --show-toplevel </dev/null 2>/dev/null && builtin printf '.')"; then
      _factory_root=${_factory_root%$'\n.'}
      builtin test -n "$_factory_root" || _factory_root=.
    else
      _factory_root=.
    fi
    # Match the native owner's relative and absolute root semantics.
    # docs/adr/0138-go-source-check-orchestration.md:99.
    case "$_factory_root" in
      /*) _factory_cd=-P ;;
      *) _factory_cd=-L ;;
    esac
    ;;
  *)
    _factory_root=${_factory_source_path%/*}
    builtin test "$_factory_root" != "$_factory_source_path" || _factory_root=.
    _factory_root="$_factory_root/.."
    _factory_cd=-P
    ;;
esac
_factory_root="$(CDPATH='' builtin cd "$_factory_cd" -- "$_factory_root" && builtin pwd -P && builtin printf '.')" || exit 2
_factory_root=${_factory_root%$'\n.'}
builtin test -f "$_factory_root/factory" && builtin test -x "$_factory_root/factory" || {
  builtin printf '%s\n' 'factory check: selected owner must be an executable regular file' >&2
  exit 2
}
exec "$_factory_root/factory" check "$@"
