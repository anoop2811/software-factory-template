#!/bin/sh
set -eu

# softwareaifactory.sh installer.
#
#   curl -fsSL https://softwareaifactory.sh/install.sh | sh
#       Fetch only. Clones the template into $FACTORY_HOME and stops.
#
#   curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- init
#       Fetch, then run factory-init against the CURRENT directory. Use this
#       from inside the repository you want to govern. The 'init' word is your
#       explicit consent to modify the current directory — the bare command
#       never touches your project.
#
#   curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- upgrade
#       Refresh the machine-wide template cache, then upgrade the repo you're in
#       (a reviewable diff — nothing committed), just like 'init' acts on the
#       current directory. Upgrades THIS repo, not every repo on the machine:
#       each repo owns its committed, governance-gated framework files. Locally,
#       './factory upgrade' does the same thing without curl.
#
#   curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- init --ref main
#       Same as 'init' (or 'upgrade'), but install from the given branch or tag
#       instead of the pinned release — '--ref main' tracks the latest. Works
#       with the bare fetch too. Equivalent to the FACTORY_REF env var, but
#       pipe-safe: the flag can't be mis-attached to curl instead of sh.
#
# What the fetch does, in full:
#   1. Clones the template (shallow, at a pinned ref) into $FACTORY_HOME
#      (default: ~/.software-factory-template)
#   2. Prints the factory-init command (or runs it, with 'init')
#
# What it never does: use sudo, phone home, or execute anything it downloaded
# other than this template's own factory-init when you ask for it.
#
# Prefer to inspect first? So would we — that is rather the point of the
# template. Download this file, read it, then run it:
#   curl -fsSLO https://softwareaifactory.sh/install.sh && less install.sh && sh install.sh
#
# Pinning: the default ref is the pinned tag below (reproducible installs). The
# `--ref <ref>` flag or the FACTORY_REF env var overrides it — e.g. main for the
# latest. From the first tagged release onward the default is that tag, never a
# moving branch.

FACTORY_REPO="${FACTORY_REPO:-https://github.com/anoop2811/software-factory-template}"
FACTORY_REF="${FACTORY_REF:-v0.1.1}"
FACTORY_HOME="${FACTORY_HOME:-$HOME/.software-factory-template}"

# --ref <ref> (or --ref=<ref>): install from the given branch or tag instead of
# the pinned default — e.g. `init --ref main` for the latest. Pulled out of the
# args here so it works before or after the verb, and passes nothing extra to
# factory-init. Precedence: --ref beats the FACTORY_REF env var beats the default.
REF_OVERRIDE=""
_args=""
while [ $# -gt 0 ]; do
  case "$1" in
    --ref)
      if [ $# -lt 2 ]; then
        printf '%s\n' "install: --ref needs a value (e.g. --ref main)" >&2
        exit 2
      fi
      REF_OVERRIDE="$2"; shift 2 ;;
    --ref=*) REF_OVERRIDE="${1#--ref=}"; shift ;;
    *) _args="$_args $1"; shift ;;
  esac
done
# shellcheck disable=SC2086  # deliberate re-split; passthrough args carry no spaces
set -- $_args
[ -n "$REF_OVERRIDE" ] && FACTORY_REF="$REF_OVERRIDE"

DO_INIT=0
DO_UPGRADE=0
case "${1:-}" in
  init) DO_INIT=1; shift ;;   # remaining args (e.g. --pack go) pass to factory-init
  upgrade) DO_UPGRADE=1; shift ;;   # refresh the machine-wide template cache
  "") : ;;
  *)
    printf '%s\n' "install: unknown argument '$1' (did you mean 'init' or 'upgrade'?)" >&2
    exit 2
    ;;
esac

# The directory to govern is where the user invoked the command — captured
# before anything else, since factory-init modifies it.
TARGET_DIR="$PWD"

say() { printf '%s\n' "$*"; }

if ! command -v git >/dev/null 2>&1; then
  say "install: git is required and was not found." >&2
  exit 1
fi

# upgrade: refresh the machine-wide template cache, then apply the framework
# update to the repo you're in (a reviewable diff — nothing is committed). Like
# 'init', it acts on the current directory. It upgrades THIS repo, not every
# repo on the machine — each repo owns its committed, governance-gated framework
# files, so you upgrade them where you are.
if [ "$DO_UPGRADE" -eq 1 ]; then
  if [ -d "$FACTORY_HOME/.git" ]; then
    say "install: refreshing the template at $FACTORY_HOME to '$FACTORY_REF'..."
    git -C "$FACTORY_HOME" fetch --quiet --depth 1 origin "$FACTORY_REF"
    git -C "$FACTORY_HOME" reset --quiet --hard FETCH_HEAD
  elif [ -e "$FACTORY_HOME" ]; then
    say "install: $FACTORY_HOME exists but is not a template checkout." >&2
    say "  Remove it and re-run, or set FACTORY_HOME to a different path." >&2
    exit 1
  else
    say "install: cloning the template into $FACTORY_HOME..."
    git clone --quiet --depth 1 --branch "$FACTORY_REF" "$FACTORY_REPO" "$FACTORY_HOME"
  fi
  # Apply to the current repo. Resolve its root via git so running from a
  # subdirectory upgrades the whole repo, not just where you happen to stand.
  REPO_ROOT="$( (cd "$TARGET_DIR" && git rev-parse --show-toplevel) 2>/dev/null || true)"
  if [ -n "$REPO_ROOT" ] && [ -f "$REPO_ROOT/factory.yaml" ]; then
    say "install: applying framework updates to $REPO_ROOT ..."
    say ""
    cd "$REPO_ROOT"
    exec "$FACTORY_HOME/scripts/factory-upgrade.sh" --source "$FACTORY_HOME"
  fi
  say "install: template cache updated. This isn't a factory repo, so nothing was"
  say "  upgraded — run 'upgrade' from inside one, or 'init' to set one up."
  exit 0
fi

if [ -e "$FACTORY_HOME" ]; then
  say "install: $FACTORY_HOME already exists (reusing it)."
  say "  To update it to the latest template:"
  say "    curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- upgrade"
else
  say "install: cloning $FACTORY_REPO at ref '$FACTORY_REF' into $FACTORY_HOME"
  git clone --quiet --depth 1 --branch "$FACTORY_REF" "$FACTORY_REPO" "$FACTORY_HOME"
  say "install: done. Nothing outside $FACTORY_HOME was touched."
fi

if [ "$DO_INIT" -eq 0 ]; then
  say ""
  say "Next, from the repository you want to govern:"
  say ""
  say "    cd your-project && \"$FACTORY_HOME/scripts/factory-init.sh\""
  say ""
  say "Or re-run this installer with 'init' from inside that repository:"
  say "    curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- init"
  exit 0
fi

say ""
say "install: running factory-init against the current directory:"
say "    $TARGET_DIR"
say ""
# factory-init reads its prompts from /dev/tty, so it works through the pipe.
# Any remaining args (e.g. --pack go) pass straight through.
exec "$FACTORY_HOME/scripts/factory-init.sh" "$TARGET_DIR" "$@"
