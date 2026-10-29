#!/bin/bash
# Native Go source adapter; docs/adr/0108-default-go-cutover.md:86.
set -e
_factory_root=${BASH_SOURCE[0]%/*}
[ "$_factory_root" != "${BASH_SOURCE[0]}" ] || _factory_root=.
_factory_root="$(CDPATH='' cd -- "$_factory_root/.." && pwd -P && printf '.')" || exit
_factory_root=${_factory_root%??}
exec "$_factory_root/factory" sync codex
