#!/bin/sh
# scripts/lib/events.sh
# factory_log_event <gate> <reason>: best-effort record of a gate firing (a
# block that a deterministic hook made), for `factory report`. Writes one
# tab-separated line to the event log — $FACTORY_EVENT_LOG if set, else
# .factory/events.log at the repo root.
#
# It must never fail a hook: a hook's job is enforcement, not bookkeeping, so
# every error here is swallowed and the function always returns 0. If the repo
# root can't be resolved (not a git checkout), it quietly does nothing.
factory_log_event() {
  _log="${FACTORY_EVENT_LOG:-}"
  if [ -z "$_log" ]; then
    _root=$(git rev-parse --show-toplevel 2>/dev/null) || return 0
    _log="$_root/.factory/events.log"
  fi
  mkdir -p "$(dirname "$_log")" 2>/dev/null || return 0
  _ts=$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || printf '?')
  printf '%s\t%s\t%s\n' "$_ts" "${1:-gate}" "${2:-blocked}" >> "$_log" 2>/dev/null || true
  return 0
}
