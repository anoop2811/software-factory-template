#!/bin/sh
# scripts/lib/hookspath.sh
# hookspath_status <repo-root> — what Git will ACTUALLY run for pre-push.
#
# A populated .githooks/pre-push is not evidence that Git will execute it. When
# core.hooksPath is set — commonly inherited from global or system config — Git
# resolves hooks from that directory and ignores the repository's, leaving an
# installed-looking push gate completely inert. So ask Git, don't assume.
#
# Echoes "<state>\t<resolved-path>", where state is one of:
#   armed    — Git resolves pre-push to this repo's tracked .githooks
#   hijacked — core.hooksPath points elsewhere; the repo's gate never runs
#   absent   — no core.hooksPath; the gate is simply not installed yet
hookspath_status() {
  _hp_root="$1"
  _hp_resolved="$(git -C "$_hp_root" rev-parse --git-path hooks/pre-push 2>/dev/null || true)"
  case "$_hp_resolved" in
    /*) ;;
    *) _hp_resolved="$_hp_root/$_hp_resolved" ;;
  esac
  _hp_cfg="$(git -C "$_hp_root" config --get core.hooksPath 2>/dev/null || true)"
  if [ "$_hp_resolved" = "$_hp_root/.githooks/pre-push" ]; then
    printf 'armed\t%s' "$_hp_resolved"
  elif [ -n "$_hp_cfg" ]; then
    printf 'hijacked\t%s' "$_hp_resolved"
  else
    printf 'absent\t%s' "$_hp_resolved"
  fi
}
