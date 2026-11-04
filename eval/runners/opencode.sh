#!/bin/sh
# Factory native evaluation runner adapter: source-v1
# docs/adr/0133-go-source-evaluation-runners.md:18.
set -e
# A failed exec can bypass EXIT traps. docs/adr/0133-go-source-evaluation-runners.md:197.
if [ "${FACTORY_EVAL_OWNERSHIP_TOKEN+x}" = x ] && [ "${#FACTORY_EVAL_OWNERSHIP_TOKEN}" -eq 64 ]; then
  case "$FACTORY_EVAL_OWNERSHIP_TOKEN" in
    *[!0123456789abcdef]*) ;;
    *) printf 'factory-eval-result-v1 %s handoff-pending\n' "$FACTORY_EVAL_OWNERSHIP_TOKEN" >&2 || : ;;
  esac
fi
[ ! -L "$0" ] || {
  printf '%s\n' 'factory eval run: invoke the physical source adapter instead of a file symlink' >&2
  exit 2
}
_factory_root=${0%/*}
# docs/adr/0133-go-source-evaluation-runners.md:154.
if [ "$_factory_root" = "$0" ]; then
  [ -f "$0" ] && [ -r "$0" ] || {
    printf '%s\n' 'factory eval run: use an explicit source adapter path instead of a bare shell operand found through PATH' >&2
    exit 2
  }
  _factory_root=.
fi
_factory_root="$(CDPATH='' cd -P -- "$_factory_root/../.." && pwd -P && printf '.')" || exit
_factory_root=${_factory_root%??}
exec "$_factory_root/factory" eval run opencode "$@"
