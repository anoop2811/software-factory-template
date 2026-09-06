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
#   armed    — Git resolves pre-push to this repo's tracked .githooks, and the
#              hook is executable, so it will actually run
#   inert    — Git resolves pre-push to this repo's tracked .githooks, but the
#              file is not executable. Git skips a non-executable hook without
#              complaint, so the gate is configured and still never fires. This
#              is the state a `chmod +x` fixes, and it is reported separately
#              from `armed` precisely because the two look identical on disk.
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
  # Git silently ignores a hook that is not executable, so a path match alone is
  # not "armed" — factory doctor would report a live push gate that never fires.
  # The executable bit is the difference between a configured gate and a real one.
  if [ "$_hp_resolved" = "$_hp_root/.githooks/pre-push" ] && [ ! -x "$_hp_resolved" ]; then
    printf 'inert\t%s' "$_hp_resolved"
  elif [ "$_hp_resolved" = "$_hp_root/.githooks/pre-push" ]; then
    printf 'armed\t%s' "$_hp_resolved"
  elif [ -n "$_hp_cfg" ]; then
    printf 'hijacked\t%s' "$_hp_resolved"
  else
    printf 'absent\t%s' "$_hp_resolved"
  fi
}
