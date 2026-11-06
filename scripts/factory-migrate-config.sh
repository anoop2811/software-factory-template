#!/bin/bash
# Factory native configuration migration adapter: source-v1
# docs/adr/0136-go-source-configuration-entrypoints.md:18.
set -e
builtin test ! -L "${BASH_SOURCE[0]}" || {
  builtin printf '%s\n' 'factory migrate-config: invoke the physical source adapter instead of a file symlink' >&2
  exit 2
}
_factory_root=${BASH_SOURCE[0]%/*}
builtin test "$_factory_root" != "${BASH_SOURCE[0]}" || _factory_root=.
_factory_root="$(CDPATH='' builtin cd -P -- "$_factory_root/.." && builtin pwd -P && builtin printf '.')" || exit
_factory_root=${_factory_root%$'\n.'}
exec "$_factory_root/factory" migrate-config "$@"
