#!/bin/bash
# Factory native budget adapter: source-v1
# docs/adr/0139-go-source-budget-entrypoint.md:16.
_factory_source_declaration="$(builtin declare -p BASH_SOURCE 2>/dev/null)"
case "${_factory_source_declaration%% BASH_SOURCE=*}" in
  'declare -'*a*) ;;
  *)
    builtin printf '%s\n' 'factory budget: native Bash source metadata is required' >&2
    exit 2
    ;;
esac
_factory_source_path=${BASH_SOURCE[0]}
builtin test ! -L "$_factory_source_path" || {
  builtin printf '%s\n' 'factory budget: invoke the physical source adapter instead of a file symlink' >&2
  exit 2
}
_factory_root=${_factory_source_path%/*}
builtin test "$_factory_root" != "$_factory_source_path" || _factory_root=.
_factory_root="$(CDPATH='' builtin cd -P -- "$_factory_root/.." && builtin pwd -P && builtin printf '.')" || exit 2
_factory_root=${_factory_root%$'\n.'}
builtin test -f "$_factory_root/factory" && builtin test -x "$_factory_root/factory" || {
  builtin printf '%s\n' 'factory budget: selected owner must be an executable regular file' >&2
  exit 2
}
exec "$_factory_root/factory" budget "$@"
