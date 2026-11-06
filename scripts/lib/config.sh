#!/bin/bash
# Factory native configuration reader: source-v1
# docs/adr/0136-go-source-configuration-entrypoints.md:33.
_factory_source_config_load() {
  builtin unset -f _factory_source_config_load
  builtin local _factory_source_root _factory_source_declaration
  # docs/adr/0137-go-source-shared-helpers.md:123.
  _factory_source_declaration="$(builtin declare -p BASH_SOURCE 2>/dev/null)"
  case "${_factory_source_declaration%% BASH_SOURCE=*}" in
    'declare -'*a*) ;;
    *)
      builtin printf '%s\n' 'factory config: native Bash source metadata is required' >&2
      return 2
      ;;
  esac
  builtin test ! -L "${BASH_SOURCE[0]}" || {
    builtin printf '%s\n' 'factory config: source the physical library instead of a file symlink' >&2
    return 2
  }
  _factory_source_root=${BASH_SOURCE[0]%/*}
  builtin test "$_factory_source_root" != "${BASH_SOURCE[0]}" || _factory_source_root=.
  _factory_source_root="$(CDPATH='' builtin cd -P -- "$_factory_source_root/../.." && builtin pwd -P && builtin printf '.')" || return
  _factory_source_root=${_factory_source_root%$'\n.'}
  # shellcheck source=../../runtime/shell/readers.sh
  builtin . "$_factory_source_root/runtime/shell/readers.sh" || return
  # shellcheck source=../../runtime/shell/config.sh
  builtin . "$_factory_source_root/runtime/shell/config.sh" || return
  # shellcheck source=../../runtime/shell/source.sh
  builtin . "$_factory_source_root/runtime/shell/source.sh"
}
_factory_source_config_load || return
