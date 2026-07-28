#!/bin/bash
set -euo pipefail

# scripts/factory-review-lane.sh
# Turns the advisory adversarial review lane on or off, and reports what it would
# cost you before you agree to it.
#
# Usage: factory review-lane [status|enable|disable]
#
# Enabling installs .github/workflows/adversarial-review.yml and records the
# choice in factory.config. Disabling removes the workflow file rather than
# leaving it inert: a dormant pull_request_target workflow in a repository is an
# invitation to switch on something nobody read.

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT" || exit 1
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEMPLATE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CONFIG="$ROOT/factory.config"
WORKFLOW="$ROOT/.github/workflows/adversarial-review.yml"
SOURCE_YML="$TEMPLATE_DIR/packs/review-lane/review-pr.yml"

# shellcheck source=/dev/null
[ -f "$CONFIG" ] && . "$CONFIG"

CMD="${1:-status}"

set_key() {
  local key="$1" val="$2"
  [ -f "$CONFIG" ] || { echo "review-lane: no factory.config here — run factory init first." >&2; exit 1; }
  if grep -q "^${key}=" "$CONFIG"; then
    sed -i.bak "s|^${key}=.*|${key}=\"${val}\"|" "$CONFIG" && rm -f "$CONFIG.bak"
  else
    printf '%s="%s"\n' "$key" "$val" >> "$CONFIG"
  fi
}

default_secret_for_provider() {
  case "${MODEL_PROVIDER:-openrouter}" in
    anthropic) printf 'ANTHROPIC_API_KEY' ;;
    openai)    printf 'OPENAI_API_KEY' ;;
    *)         printf 'OPENROUTER_API_KEY' ;;
  esac
}

case "$CMD" in
  status)
    echo "review lane: ${REVIEW_LANE:-off}"
    echo "  model:     ${REVIEW_MODEL:-<frontier tier for ${MODEL_PROVIDER:-openrouter}>}"
    echo "  secret:    ${REVIEW_API_KEY_SECRET:-$(default_secret_for_provider)}"
    if [ -f "$WORKFLOW" ]; then
      echo "  workflow:  installed (.github/workflows/adversarial-review.yml)"
    else
      echo "  workflow:  not installed"
    fi
    ;;

  enable)
    [ -f "$SOURCE_YML" ] || { echo "review-lane: $SOURCE_YML not found" >&2; exit 1; }
    SECRET="${2:-${REVIEW_API_KEY_SECRET:-$(default_secret_for_provider)}}"
    mkdir -p "$ROOT/.github/workflows"
    sed "s|__REVIEW_API_KEY_SECRET__|$SECRET|g" "$SOURCE_YML" > "$WORKFLOW"
    set_key REVIEW_LANE on
    set_key REVIEW_API_KEY_SECRET "$SECRET"
    echo "review lane: enabled."
    echo ""
    echo "  It runs a model over the diff of every pull request and posts an"
    echo "  advisory comment. That costs tokens on each PR — the reviewer is"
    echo "  deliberately the frontier tier, so it is the expensive one."
    echo ""
    echo "  One step is yours, and the lane cannot work without it:"
    echo "    add a repository secret named $SECRET"
    echo "    (Settings -> Secrets and variables -> Actions -> New repository secret)"
    echo ""
    echo "  It is advisory only and never a required check. Turn it off any time"
    echo "  with: ./factory review-lane disable"
    ;;

  disable)
    rm -f "$WORKFLOW"
    set_key REVIEW_LANE off
    echo "review lane: disabled (workflow removed; nothing runs on your PRs)."
    ;;

  *)
    echo "usage: factory review-lane [status|enable|disable]" >&2
    exit 2
    ;;
esac
