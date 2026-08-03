#!/bin/sh
# scripts/lib/color.sh
# Emphasis for things the reader must act on — nothing decorative.
#
# Colour only when stdout is a terminal and NO_COLOR is unset: escape codes in a
# piped log, a CI transcript, or a file are noise pretending to be emphasis. The
# variables are always defined, so callers never guard their use.
# shellcheck disable=SC2034  # a palette: not every caller uses every colour
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
  C_RESET="$(printf '\033[0m')"
  C_BOLD="$(printf '\033[1m')"
  C_DIM="$(printf '\033[2m')"
  C_YELLOW="$(printf '\033[33m')"
  C_GREEN="$(printf '\033[32m')"
  C_RED="$(printf '\033[31m')"
else
  C_RESET=""; C_BOLD=""; C_DIM=""; C_YELLOW=""; C_GREEN=""; C_RED=""
fi

# action_box <title> <line>... — a bordered, highlighted block for work the
# reader still has to do. Used at the END of a run, where it cannot scroll away.
action_box() {
  _ab_title="$1"; shift
  printf '%s\n' "${C_YELLOW}${C_BOLD}┌─ $_ab_title ${C_RESET}"
  for _ab_line in "$@"; do
    printf '%s\n' "${C_YELLOW}│${C_RESET} $_ab_line"
  done
  printf '%s\n' "${C_YELLOW}└─${C_RESET}"
}
