#!/bin/sh
# scripts/lib/timing.sh
# How long things took, reported the same way everywhere.
#
# Two commands here run long enough that an adopter wonders whether they have
# stalled — the upgrade, and the doctor's break/fix proof. A duration at the end
# turns "did that hang?" into a number, and a number you can compare next time.
#
# POSIX sh: `date +%s` rather than bash's SECONDS, because install.sh is /bin/sh.
# Seconds resolution is deliberate — these are minute-scale operations, and a
# stopwatch precise to the millisecond would imply a measurement it is not.

# factory_now: epoch seconds, or 0 where date cannot produce them. Timing must
# never be the reason a command fails, so every path here degrades to a zero
# rather than an error.
factory_now() {
  date +%s 2>/dev/null || printf '0'
}

# factory_duration <start-epoch> [end-epoch] -> "4s" | "1m 12s" | "1h 3m"
# Rounded to what a reader acts on: sub-minute in seconds, minutes with seconds,
# hours with minutes. Nobody needs "3847s".
factory_duration() {
  _fd_start="${1:-0}"
  _fd_end="${2:-$(factory_now)}"
  case "$_fd_start" in ''|*[!0-9]*) printf 'unknown'; return 0 ;; esac
  case "$_fd_end" in ''|*[!0-9]*) printf 'unknown'; return 0 ;; esac
  _fd_secs=$((_fd_end - _fd_start))
  # A clock that moved backwards (NTP, a suspend) is not a negative duration.
  [ "$_fd_secs" -lt 0 ] && _fd_secs=0

  if [ "$_fd_secs" -lt 60 ]; then
    printf '%ss' "$_fd_secs"
  elif [ "$_fd_secs" -lt 3600 ]; then
    printf '%dm %ds' "$((_fd_secs / 60))" "$((_fd_secs % 60))"
  else
    printf '%dh %dm' "$((_fd_secs / 3600))" "$(((_fd_secs % 3600) / 60))"
  fi
}
