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
# Pinning: FACTORY_REF selects the git ref (default below). From the first
# tagged release onward the default is that tag, never a moving branch.

FACTORY_REPO="${FACTORY_REPO:-https://github.com/anoop2811/software-factory-template}"
FACTORY_REF="${FACTORY_REF:-main}"
FACTORY_HOME="${FACTORY_HOME:-$HOME/.software-factory-template}"

DO_INIT=0
case "${1:-}" in
  init) DO_INIT=1; shift ;;   # remaining args (e.g. --pack go) pass to factory-init
  "") : ;;
  *)
    printf '%s\n' "install: unknown argument '$1' (did you mean 'init'?)" >&2
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

if [ -e "$FACTORY_HOME" ]; then
  say "install: $FACTORY_HOME already exists (reusing it)."
  say "  To update the template: rm -rf \"$FACTORY_HOME\" and run this installer again."
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
