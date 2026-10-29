#!/bin/bash
# Native Go source adapter; docs/adr/0108-default-go-cutover.md:86.
set -e
_factory_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
exec "$_factory_root/factory" upgrade "$@"
