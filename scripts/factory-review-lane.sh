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
WORKFLOW="$ROOT/.github/workflows/adversarial-review.yml"
SOURCE_YML="$TEMPLATE_DIR/packs/review-lane/review-pr.yml"

# shellcheck source=lib/color.sh
[ -f "$SCRIPT_DIR/lib/color.sh" ] && . "$SCRIPT_DIR/lib/color.sh"
# The lib is optional: emphasis must never be why a command fails.
command -v action_box >/dev/null 2>&1 || action_box() { printf '%s\n' "== $1 =="; shift; for _l in "$@"; do printf '  %s\n' "$_l"; done; }
# Settings live in factory.yaml and are parsed, not sourced (Decision 41).
# shellcheck source=lib/config.sh
. "$SCRIPT_DIR/lib/config.sh"
factory_config_export

CMD="${1:-status}"

# The lane's settings are lowercase factory.yaml keys; the shell variables the
# rest of this script reads are their exported upper-case forms.
set_key() {
  factory_config_set "$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')" "$2" || exit 1
}


# secret_status -> set | missing | unknown | n/a
# The lane cannot work without the repository secret, and the adopter is the only
# one who can add it. Ask GitHub when we can; say "unknown" rather than guess.
secret_status() {
  [ "${REVIEW_LANE:-off}" = "on" ] || { printf 'n/a'; return 0; }
  command -v gh >/dev/null 2>&1 || { printf 'unknown'; return 0; }
  # One call, reused: listing twice doubles the API round trips and the latency
  # for no gain, and a failure here means "cannot tell", not "missing".
  if ! _ss_list="$(gh secret list 2>/dev/null)"; then
    printf 'unknown'; return 0
  fi
  if printf '%s\n' "$_ss_list" | awk '{print $1}' | grep -qx "$(effective_secret_name)"; then
    printf 'set'
  else
    printf 'missing'
  fi
}

# The name every message must agree on: explicit config, else the provider's
# default. Reporting "unset" while the lane falls back to a real name would send
# the adopter to add the wrong secret.
effective_secret_name() {
  printf '%s' "${REVIEW_API_KEY_SECRET:-$(default_secret_for_provider)}"
}

default_model_for_provider() {
  case "${MODEL_PROVIDER:-openrouter}" in
    anthropic) printf '%s' "${CLAUDE_FRONTIER_MODEL:-claude-opus-4-8}" ;;
    openai)    printf '%s' "${CODEX_FRONTIER_MODEL:-gpt-5.6-sol}" ;;
    *)         printf '%s' "${OPENCODE_FRONTIER_MODEL:-openrouter/z-ai/glm-5.2}" ;;
  esac
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
    if [ ! -f "$SOURCE_YML" ]; then
      echo "review-lane: $SOURCE_YML not found." >&2
      echo "  This repo predates the review lane. Pull it in with:" >&2
      echo "    curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- upgrade" >&2
      exit 1
    fi
    SECRET="${2:-${REVIEW_API_KEY_SECRET:-$(default_secret_for_provider)}}"
    # Which model reviews. Blank means "resolve the frontier tier at run time",
    # which is the right default — but it is the adopter's money, so ask when
    # there is someone to ask and nothing has been chosen yet.
    if [ -z "${REVIEW_MODEL:-}" ] && [ -r /dev/tty ] && [ -t 1 ]; then
      echo "Review model — the reviewer runs at the frontier tier by default."
      echo "  provider: ${MODEL_PROVIDER:-openrouter}"
      echo "  default:  $(default_model_for_provider)"
      printf '  Model to use (Enter for the default): '
      read -r REVIEW_MODEL_ANSWER < /dev/tty || REVIEW_MODEL_ANSWER=""
      REVIEW_MODEL="$REVIEW_MODEL_ANSWER"
    fi
    mkdir -p "$ROOT/.github/workflows"
    sed "s|__REVIEW_API_KEY_SECRET__|$SECRET|g" "$SOURCE_YML" > "$WORKFLOW"
    set_key REVIEW_LANE on
    set_key REVIEW_API_KEY_SECRET "$SECRET"
    set_key REVIEW_MODEL "${REVIEW_MODEL:-}"
    echo "review lane: enabled."
    echo "  model:  ${REVIEW_MODEL:-$(default_model_for_provider) (frontier tier)}"
    echo ""
    echo "  It runs a model over the diff of every pull request and posts an"
    echo "  advisory comment. That costs tokens on each PR — the reviewer is"
    echo "  deliberately the frontier tier, so it is the expensive one."
    echo ""
    action_box "Action required — the lane cannot work without this" \
      "Add a repository secret named ${C_BOLD:-}${SECRET}${C_RESET:-}" \
      "GitHub -> Settings -> Secrets and variables -> Actions -> New repository secret" \
      "Value: your ${MODEL_PROVIDER:-openrouter} API key"
    echo ""
    echo "  It is advisory only and never a required check. Turn it off any time"
    echo "  with: ./factory review-lane disable"
    ;;

  disable)
    rm -f "$WORKFLOW"
    set_key REVIEW_LANE off
    echo "review lane: disabled (workflow removed; nothing runs on your PRs)."
    ;;

  secret-name)
    effective_secret_name; echo
    ;;

  pending)
    # One line per outstanding action; silence means nothing to do.
    if [ "${REVIEW_LANE:-off}" = "on" ]; then
      SEC="$(effective_secret_name)"
      case "$(secret_status)" in
        missing|unknown)
          echo "The adversarial review lane is ON but needs a repository secret:"
          echo "  add a secret named $SEC"
          echo "  GitHub -> Settings -> Secrets and variables -> Actions -> New repository secret"
          echo "  Until then the lane comments to say the secret is missing."
          ;;
      esac
    fi
    ;;

  *)
    echo "usage: factory review-lane [status|enable|disable|pending|secret-name]" >&2
    exit 2
    ;;
esac
