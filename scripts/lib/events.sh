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
  case "$_max" in ''|*[!0-9]*|0) _max=5000 ;; esac
  # Keep at least one line. A tiny cap must mean "keep very little", never
  # "silently empty the log the moment it fills".
  _keep=$((_max / 2)); [ "$_keep" -lt 1 ] && _keep=1
  if [ "$(wc -l < "$_log" 2>/dev/null || echo 0)" -gt "$_max" ]; then
    # mktemp, not "$_log.trim": that name is predictable and lives in a directory
    # other processes can write, so a symlink planted there would be followed and
    # its target truncated with event data. mktemp fails rather than following.
    _tmp="$(mktemp "$(dirname "$_log")/.events.XXXXXX" 2>/dev/null)" || return 0
    if tail -n "$_keep" "$_log" > "$_tmp" 2>/dev/null; then
      mv -f "$_tmp" "$_log" 2>/dev/null || rm -f "$_tmp" 2>/dev/null || true
    else
      rm -f "$_tmp" 2>/dev/null || true
    fi
  fi
  return 0
}
