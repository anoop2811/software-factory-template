#!/bin/sh
# Factory native evaluation runner adapter: source-v1
# docs/adr/0133-go-source-evaluation-runners.md:18.
set -e
[ ! -L "$0" ] || {
  printf '%s\n' 'factory eval run: invoke the physical source adapter instead of a file symlink' >&2
  exit 2
}
_factory_root=${0%/*}
# docs/adr/0133-go-source-evaluation-runners.md:154.
if [ "$_factory_root" = "$0" ]; then
  [ -f "$0" ] && [ -r "$0" ] || {
    printf '%s\n' 'factory eval run: use an explicit source adapter path instead of a bare shell operand found through PATH' >&2
    exit 2
  }
  _factory_root=.
fi
_factory_root="$(CDPATH='' cd -P -- "$_factory_root/../.." && pwd -P && printf '.')" || exit
_factory_root=${_factory_root%??}
exec "$_factory_root/factory" eval run claude "$@"
