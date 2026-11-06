#!/bin/bash
# Source-only transport; the canonical bridges retain all shell plan semantics.
# docs/adr/0136-go-source-configuration-entrypoints.md:33.
_factory_source_call() (
  builtin local _factory_source_root
  _factory_source_root="$(CDPATH='' builtin cd -P -- "${BASH_SOURCE[0]%/*}/../.." && builtin pwd -P && builtin printf '.')" || return
  _factory_source_root=${_factory_source_root%$'\n.'}
  # Only the child receives controls, including caller-local readonly values.
  # docs/adr/0136-go-source-configuration-entrypoints.md:49.
  if builtin test "${1-}" = helper; then
    exec /usr/bin/env "FACTORY_CONFIG=${FACTORY_CONFIG-}" "NO_COLOR=${NO_COLOR-}" \
      "FACTORY_EVENT_LOG=${FACTORY_EVENT_LOG-}" "FACTORY_EVENT_MAX_LINES=${FACTORY_EVENT_MAX_LINES-}" \
      FACTORY_BRIDGE_PROTOCOL=1 "$_factory_source_root/factory" "$@"
  fi
  exec /usr/bin/env "FACTORY_CONFIG=${FACTORY_CONFIG-}" FACTORY_BRIDGE_PROTOCOL=1 \
    "$_factory_source_root/factory" "$@"
)

_factory_reader_call() { _factory_source_call "$@"; }
_factory_config_call() { _factory_source_call "$@"; }
_factory_helper_call() { _factory_source_call helper "$@"; }
