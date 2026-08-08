#!/bin/bash
set -euo pipefail

# scripts/sync-opencode.sh
# Applies the per-tier opencode models from factory.yaml to opencode.json and
# the .opencode/agent/*.md role files, so reconfiguring is one factory.yaml
# edit plus `make sync-harnesses` — the same flow as Claude and Codex.
#
# It writes the top-level model (default tier), small_model (economy tier), and
# each agent's model (by role tier). The COST_PROFILE collapse is applied here,
# at sync time, so flipping the profile in factory.yaml and re-syncing works.
#
# In the template repo these keys are unset, so OPENCODE_*_MODEL stay unset
# and this script leaves opencode.json / the role files untouched — the committed
# placeholders stay put and the drift check stays clean.

if ! command -v jq >/dev/null 2>&1; then
  echo "ERROR: sync-opencode requires jq" >&2
  exit 1
fi

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
OPENCODE_JSON="$ROOT_DIR/opencode.json"

if [ ! -f "$OPENCODE_JSON" ]; then
  echo "ERROR: opencode.json not found at $OPENCODE_JSON" >&2
  exit 1
fi

# Settings come from factory.yaml, parsed rather than sourced (Decision 41);
# older repos keep theirs in factory.config and are still read.
# shellcheck source=lib/config.sh
. "$ROOT_DIR/scripts/lib/config.sh"
factory_config_export
# shellcheck source=lib/roles.sh
. "$ROOT_DIR/scripts/lib/roles.sh"

# The template repo itself has no configured harness settings: leave the
# committed placeholders alone so the repo stays drift-clean. The signal is the
# absence of the keys rather than the absence of a file — since Decision 41 they
# live in factory.yaml, which every factory repo has, including this one.
if ! factory_config_has cost_profile && [ ! -f "$ROOT_DIR/factory.config" ]; then
  echo "sync-opencode: no harness settings configured — leaving opencode.json as-is"
  exit 0
fi

# A configured repo with blank tiers means "inherit": remove every model pin so
# opencode falls back to its own configuration. Stripping matters — leaving an
# unresolved __DEFAULT_MODEL__ placeholder behind would be read as a model name.
if [ -z "${OPENCODE_DEFAULT_MODEL:-}" ]; then
  TMP="$OPENCODE_JSON.sync-tmp.$$"
  jq 'del(.model, .small_model) | (.agent // {}) |= with_entries(.value |= del(.model))' \
    "$OPENCODE_JSON" > "$TMP" && mv -f "$TMP" "$OPENCODE_JSON"
  for ROLE_FILE in "$ROOT_DIR/.opencode/agent/"*.md; do
    [ -f "$ROLE_FILE" ] || continue
    sed -i.bak '/^model:[[:space:]]*/d' "$ROLE_FILE"
    rm -f "$ROLE_FILE.bak"
  done
  echo "sync-opencode: tiers blank (inherit) — model pins removed; opencode uses its own"
  exit 0
fi

PROFILE="${COST_PROFILE:-standard}"

# opencode_model <tier> -> the model for that tier after the profile collapse.
# A blank or unset frontier/economy value falls back to the default-tier model
# (which is required to be set, since the script exits early without it) — the
# same "blank tier collapses" behaviour sync-claude / sync-codex use.
opencode_model() {
  eff=$(resolve_tier "$PROFILE" "$1")
  case "$eff" in
    frontier) printf '%s' "${OPENCODE_FRONTIER_MODEL:-$OPENCODE_DEFAULT_MODEL}" ;;
    economy)  printf '%s' "${OPENCODE_ECONOMY_MODEL:-$OPENCODE_DEFAULT_MODEL}" ;;
    *)        printf '%s' "${OPENCODE_DEFAULT_MODEL}" ;;
  esac
}

# Top-level model is the default tier; small_model (background tasks) is economy.
MAIN_MODEL=$(opencode_model default)
SMALL_MODEL=$(opencode_model economy)
TMP="$OPENCODE_JSON.sync-tmp.$$"
jq --arg m "$MAIN_MODEL" --arg s "$SMALL_MODEL" '.model = $m | .small_model = $s' \
  "$OPENCODE_JSON" > "$TMP" && mv -f "$TMP" "$OPENCODE_JSON"

for AGENT_NAME in $(jq -r '.agent // {} | keys[]' "$OPENCODE_JSON"); do
  TIER=$(role_tier "$AGENT_NAME")
  MODEL=$(opencode_model "$TIER")
  jq --arg a "$AGENT_NAME" --arg m "$MODEL" '.agent[$a].model = $m' \
    "$OPENCODE_JSON" > "$TMP" && mv -f "$TMP" "$OPENCODE_JSON"
  ROLE_FILE="$ROOT_DIR/.opencode/agent/${AGENT_NAME}.md"
  if [ -f "$ROLE_FILE" ]; then
    sed -i.bak "s|^model:.*|model: $MODEL|" "$ROLE_FILE"
    rm -f "$ROLE_FILE.bak"
  fi
  echo "sync-opencode: ${AGENT_NAME} -> ${MODEL}"
done

echo "sync-opencode: model=${MAIN_MODEL} small_model=${SMALL_MODEL}"
echo "sync-opencode: done"
