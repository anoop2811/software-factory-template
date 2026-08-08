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
  # Bounded, cheaply. A gate block is one short line and blocks are rare, so this
  # trims only after thousands of them — no database, no rotation scheme. Older
  # raw events are dropped, so `factory metrics` reports blocks RETAINED rather
  # than claiming an all-time total it can no longer stand behind.
  _max="${FACTORY_EVENT_MAX_LINES:-5000}"
  case "$_max" in ''|*[!0-9]*) _max=5000 ;; esac
  if [ "$(wc -l < "$_log" 2>/dev/null || echo 0)" -gt "$_max" ]; then
    tail -n "$((_max / 2))" "$_log" > "$_log.trim" 2>/dev/null &&
      mv -f "$_log.trim" "$_log" 2>/dev/null || true
  fi
  return 0
}
