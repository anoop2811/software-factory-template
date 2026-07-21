#!/bin/bash
set -euo pipefail

# scripts/factory-report.sh
# A cost report in three honest registers: facts the factory computed itself (0
# model tokens), one clearly-labeled estimate, and a pointer to where measured
# token spend actually lives (the harness). It never prints a "tokens saved"
# headline — that is a counterfactual it cannot measure. See docs/COST_AND_TOKENS.md.
#
# Usage: factory report            # print the report
#        factory report --clear    # reset the gate-block event log

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
EVENT_LOG="${FACTORY_EVENT_LOG:-$DIR/.factory/events.log}"

if [ "${1:-}" = "--clear" ]; then
  rm -f "$EVENT_LOG" 2>/dev/null || true
  echo "factory report: event log cleared."
  exit 0
fi

# Model routing is a fact read from factory.config (absent in the template repo).
# shellcheck source=/dev/null
[ -f "$DIR/factory.config" ] && . "$DIR/factory.config"

# Rough token cost of one LLM review pass, for the estimate only. Overridable.
R_TOKENS="${FACTORY_REVIEW_TOKENS:-3000}"

GATES=$(find "$DIR/scripts/hooks" -maxdepth 1 -name '*.sh' 2>/dev/null | wc -l | tr -d ' ')
BLOCKS=0
[ -f "$EVENT_LOG" ] && BLOCKS=$(grep -c . "$EVENT_LOG" 2>/dev/null || echo 0)

echo "Factory report"
echo ""
echo "Facts (deterministic, 0 model tokens)"
echo "  Deterministic gates installed:  $GATES  (run 'factory doctor' for armed/inert)"
[ -n "${COST_PROFILE:-}" ] && echo "  Cost profile:                   ${COST_PROFILE}"
if [ -n "${OPENCODE_DEFAULT_MODEL:-}" ]; then
  echo "  opencode tiers:                 frontier=${OPENCODE_FRONTIER_MODEL:-?} default=${OPENCODE_DEFAULT_MODEL} economy=${OPENCODE_ECONOMY_MODEL:-?}"
fi
echo "  Gate blocks recorded:           $BLOCKS"
if [ "$BLOCKS" -gt 0 ]; then
  while IFS="$(printf '\t')" read -r ts gate reason; do
    [ -n "$gate" ] && echo "    - $ts  $gate  $reason"
  done < "$EVENT_LOG"
  echo "  (run 'factory report --clear' to reset this window)"
fi
echo ""
echo "Estimate (labeled)"
if [ "$BLOCKS" -gt 0 ]; then
  echo "  Review-spend avoided:  ~$((BLOCKS * R_TOKENS)) tokens"
  echo "    = $BLOCKS block(s) x ~$R_TOKENS tokens per LLM review pass (R; set FACTORY_REVIEW_TOKENS)."
  echo "    Each block is a catch a model reviewer would have had to make."
  echo "    Method: docs/COST_AND_TOKENS.md."
else
  echo "  No blocks recorded yet — nothing to estimate. The gates still run on every"
  echo "  commit and push at 0 model tokens; an LLM reviewer would cost ~$R_TOKENS per pass."
fi
echo ""
echo "Measured token spend"
echo "  The factory does not meter tokens — your harness does. See its usage output;"
echo "  opencode, Claude Code, and Codex each report per-session token counts."
echo ""
echo "Not shown: a \"tokens saved\" headline. That compares this run to one that never"
echo "happened. A real saved figure comes from an A/B eval (docs/COST_AND_TOKENS.md)."
