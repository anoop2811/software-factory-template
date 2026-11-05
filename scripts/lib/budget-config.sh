#!/bin/bash
# Factory native budget configuration reader: source-v1
# docs/adr/0136-go-source-configuration-entrypoints.md:42.
_factory_source_budget_load() {
  builtin local _factory_source_root
  builtin test ! -L "${BASH_SOURCE[0]}" || {
    builtin printf '%s\n' 'factory budget: source the physical configuration library instead of a file symlink' >&2
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
_factory_source_budget_load || return
builtin unset -f _factory_source_budget_load
factory_budget_config
