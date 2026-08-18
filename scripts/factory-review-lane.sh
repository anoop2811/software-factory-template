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
# The first line this tool writes into the workflow, and the only thing that makes
# a file at that path ours. A substring test was not an ownership test: the shipped
# workflow body mentions `./factory review-lane disable` in its own comments, and
# so could an adopter's file, which meant `enable` would overwrite and `disable`
# would delete workflows this tool never generated.
MANAGED_HEADER='# Managed by: factory review-lane. Remove with: ./factory review-lane disable'

# factory_generated <path> — true only for a regular file whose first line is the
# marker. A symlink is never ours: writing through one would land the workflow
# wherever it points, possibly outside the repository, and removing one would
# delete the adopter's link rather than a file we created.
factory_generated() {
  [ -L "$1" ] && return 1
  [ -f "$1" ] || return 1
  [ "$(head -n 1 "$1" 2>/dev/null)" = "$MANAGED_HEADER" ]
}
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
    # Validate before writing. set_key needs factory.yaml, so writing the
    # workflow first meant a missing config left an ACTIVE pull_request_target
    # workflow behind on a command that then failed — the worst possible
    # half-state for a privileged lane.
    # Ask the config library which file it will write, so the preflight and
    # set_key agree. Hardcoding $ROOT/factory.yaml meant that with FACTORY_CONFIG
    # pointing elsewhere — a supported override — enable refused on the grounds
    # that a file it was never going to touch was missing.
    CONFIG_FILE="$(factory_config_file)"
    if [ ! -f "$CONFIG_FILE" ]; then
      echo "review lane: no $CONFIG_FILE — run 'factory init' first." >&2
      echo "  Nothing was written; the lane is still off." >&2
      exit 1
    fi
    # A workflow at this path that the factory did not generate belongs to the
    # adopter. Overwriting it silently is not this tool's call.
    if { [ -e "$WORKFLOW" ] || [ -L "$WORKFLOW" ]; } && ! factory_generated "$WORKFLOW"; then
      echo "review lane: $WORKFLOW already exists and was not generated by the factory." >&2
      echo "  Move or remove it first — refusing to overwrite a workflow you own." >&2
      exit 1
    fi
    mkdir -p "$ROOT/.github/workflows"
    # Render beside the target, record the settings, and only then move it into
    # place. The workflow is the privileged half of this command — it runs on
    # `pull_request_target` — so it must not exist for a moment while the config
    # that governs it is still being written. If a set_key fails, the EXIT trap
    # removes the staged copy and no privileged workflow was ever installed.
    STAGED="$(mktemp "$ROOT/.github/workflows/.adversarial-review.XXXXXX")" || {
      echo "review lane: cannot stage the workflow in $ROOT/.github/workflows." >&2
      exit 1
    }
    # A literal trap calling a function: `trap "rm -f '$STAGED'"` interpolated the
    # path into a string the shell re-parses when the trap fires, so a repository
    # path containing an apostrophe broke the cleanup — and a crafted one could
    # have run commands.
    _rl_cleanup() { [ -n "${STAGED:-}" ] && rm -f "$STAGED"; }
    trap _rl_cleanup EXIT INT TERM
    {
      printf '%s\n' "$MANAGED_HEADER"
      sed "s|__REVIEW_API_KEY_SECRET__|$SECRET|g" "$SOURCE_YML"
    } > "$STAGED"
    # REVIEW_LANE last. The other two keys are inert on their own — a recorded
    # secret name with the lane off does nothing — while `REVIEW_LANE on` is what
    # doctor and `pending` read as "this lane is live". Writing it first meant a
    # later failure left the lane advertised as armed with no workflow installed.
    set_key REVIEW_API_KEY_SECRET "$SECRET"
    set_key REVIEW_MODEL "${REVIEW_MODEL:-}"
    set_key REVIEW_LANE on
    # mv, not cp: the workflow appears complete or not at all, and a rename
    # replaces the name rather than following anything sitting at it.
    if ! mv -f "$STAGED" "$WORKFLOW"; then
      # Turn the lane back off rather than telling the adopter to. `REVIEW_LANE on`
      # with no workflow is a lane doctor reports as armed and that runs nothing,
      # and the command that created that state is the one that should undo it.
      echo "review lane: could not install $WORKFLOW." >&2
      # factory_config_set directly, not set_key: set_key is `... || exit 1`, so
      # using it here would leave the script and the message below could never
      # print. A rollback whose failure path is unreachable is not a rollback.
      if factory_config_set review_lane off 2>/dev/null; then
        echo "  Rolled the lane back to off; nothing is installed and nothing claims to be." >&2
      else
        echo "  The lane could not be rolled back either — run 'factory review-lane disable'." >&2
      fi
      exit 1
    fi
    chmod 644 "$WORKFLOW" 2>/dev/null || true
    STAGED=""
    trap - EXIT INT TERM
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
    # Only remove a workflow this tool generated. `rm -f` deleted whatever sat at
    # that path, including an adopter's own file of the same name.
    if { [ -e "$WORKFLOW" ] || [ -L "$WORKFLOW" ]; } && ! factory_generated "$WORKFLOW"; then
      echo "review lane: $WORKFLOW was not generated by the factory — leaving it." >&2
      set_key REVIEW_LANE off
      echo "review lane: marked off in factory.yaml; remove that file yourself if you meant to." >&2
      exit 1
    fi
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
