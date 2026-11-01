#!/bin/bash
# Factory native codex synchronization adapter: source-v1
# docs/adr/0128-go-source-harness-sync-entrypoints.md:19.
set -e
[ ! -L "${BASH_SOURCE[0]}" ] || {
  printf '%s\n' 'factory sync codex: invoke the physical source adapter instead of a file symlink' >&2
  exit 2
}
_factory_root=${BASH_SOURCE[0]%/*}
[ "$_factory_root" != "${BASH_SOURCE[0]}" ] || _factory_root=.
_factory_root="$(CDPATH='' cd -P -- "$_factory_root/.." && pwd -P && printf '.')" || exit
_factory_root=${_factory_root%$'\n.'}
exec "$_factory_root/factory" sync codex
