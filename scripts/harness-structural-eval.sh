#!/bin/bash
# Factory native structural evaluator adapter: source-v1
# docs/adr/0134-go-source-structural-evaluation.md:17.
set -e
[ ! -L "${BASH_SOURCE[0]}" ] || {
  printf '%s\n' 'factory eval structural: invoke the physical source adapter instead of a file symlink' >&2
  exit 2
}
_factory_root=${BASH_SOURCE[0]%/*}
[ "$_factory_root" != "${BASH_SOURCE[0]}" ] || _factory_root=.
_factory_root="$(CDPATH='' cd -P -- "$_factory_root/.." && pwd -P && printf '.')" || exit
_factory_root=${_factory_root%$'\n.'}
exec "$_factory_root/factory" eval structural "$@"
