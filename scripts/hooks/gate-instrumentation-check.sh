#!/bin/bash
set -uo pipefail

# scripts/hooks/gate-instrumentation-check.sh (Decision 40)
# Every gate that can block must be able to say so.
#
# `factory metrics` reports "gates installed" beside "blocks caught". A gate
# that exits non-zero without calling factory_log_event makes those two numbers
# lie together: the report shows a large installed count and a small block
# count, and the honest reading — "this gate fired and nothing recorded it" —
# is unavailable. Adopters found this the expensive way, with a repo reporting
# 14 gates, 0 blocks, and 8 of those gates structurally unable to report one.
#
# The rule: a hook with a blocking exit path calls factory_log_event on it.
#
# Scans the repo given as $1, or this repo. Checks scripts/hooks/*.sh and any
# packs/*/hooks/*.sh, since pack gates install into scripts/hooks/ and are just
# as capable of blocking a push.
#
# Exit 0 = every blocking gate is instrumented, 1 = one is mute.

ROOT="${1:-$(cd "$(dirname "$0")/../.." && pwd)}"
cd "$ROOT" || exit 1

# Some gates exit non-zero without blocking anything — a session-end nudge, say.
# Counting those as blocks would inflate the enforcement numbers, so a gate can
# opt out by carrying the marker below in a comment, next to the reason:
#
#   # factory: no-block-event — this gate's non-zero exit stops no work.
#
# The exemption lives in the gate file rather than in a list here, so the reason
# travels with the code and there is one place to change when it stops being
# true. `factory metrics` reads the same marker.
EXEMPT_MARKER='factory: no-block-event'

ERRORS=0
CHECKED=0

for HOOK in scripts/hooks/*.sh packs/*/hooks/*.sh; do
  [ -f "$HOOK" ] || continue

  grep -q "$EXEMPT_MARKER" "$HOOK" 2>/dev/null && continue

  # Does it have a blocking exit path at all? Gates that only ever exit 0 are
  # advisory by construction and have nothing to report.
  if ! grep -qE '^[[:space:]]*exit[[:space:]]+[1-9]' "$HOOK"; then
    continue
  fi
  CHECKED=$((CHECKED + 1))

  if ! grep -q 'factory_log_event' "$HOOK"; then
    echo "GATE-INSTRUMENTATION FAIL: $HOOK can block but never calls factory_log_event"
    echo "  It will stop work while 'factory metrics' reports zero blocks for it."
    ERRORS=$((ERRORS + 1))
    continue
  fi

  # Sourcing the lib is what makes the call real. Without it the gate either
  # dies on an unbound function or silently does nothing, depending on the
  # shell — and either way the event is never written.
  if ! grep -q 'lib/events.sh' "$HOOK"; then
    echo "GATE-INSTRUMENTATION FAIL: $HOOK calls factory_log_event but never sources lib/events.sh"
    ERRORS=$((ERRORS + 1))
  fi
done

if [ "$ERRORS" -gt 0 ]; then
  echo "gate-instrumentation-check: $ERRORS blocking gate(s) cannot report a block"
  exit 1
fi

echo "gate-instrumentation-check: all $CHECKED blocking gate(s) can report a block"
