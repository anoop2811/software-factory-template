#!/bin/sh
set -eu

# softwareaifactory.sh installer — fetches the template, nothing more.
#
#   curl -fsSL https://softwareaifactory.sh/install.sh | sh
#
# What this does, in full:
#   1. Clones the template (shallow, at a pinned ref) into $FACTORY_HOME
#      (default: ~/.software-factory-template)
#   2. Prints the factory-init command for you to run in your project
#
# What it deliberately does NOT do:
#   - run factory-init for you (init is interactive and belongs in your terminal)
#   - touch anything outside $FACTORY_HOME
#   - use sudo, phone home, or execute anything it downloaded
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

say() { printf '%s\n' "$*"; }

if ! command -v git >/dev/null 2>&1; then
  say "install: git is required and was not found." >&2
  exit 1
fi

if [ -e "$FACTORY_HOME" ]; then
  say "install: $FACTORY_HOME already exists."
  say "  To update: rm -rf \"$FACTORY_HOME\" and run this installer again."
  say "  To use it now:"
  say ""
  say "    cd your-project && \"$FACTORY_HOME/scripts/factory-init.sh\""
  exit 0
fi

say "install: cloning $FACTORY_REPO at ref '$FACTORY_REF' into $FACTORY_HOME"
git clone --quiet --depth 1 --branch "$FACTORY_REF" "$FACTORY_REPO" "$FACTORY_HOME"

say ""
say "install: done. Nothing was executed; nothing outside $FACTORY_HOME was touched."
say ""
say "Next, from the repository you want to govern:"
say ""
say "    cd your-project && \"$FACTORY_HOME/scripts/factory-init.sh\""
say ""
say "factory-init asks a few questions, installs the gates, and refuses to"
say "say \"done\" until it has watched every installed gate fire and pass on"
say "your machine."
