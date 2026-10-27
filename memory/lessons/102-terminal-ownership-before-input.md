# Terminal ownership must precede the child's first input

A child's PID output does not establish terminal foreground ownership. Check the
actual foreground group before injecting terminal controls. Separately, a parent
handoff after Start leaves a window in which child code can read from a background
process group. Admit eligible terminal ownership before exec; retain restoration
on failed execution and keep non-TTY/background callers outside that admission.

Canonical correction: docs/DECISION_LOG.md:5067. Go's Foreground/Ctty contract was
checked in the existing Go 1.27.3 Linux/BSD source; no dependency pin changed.

Provenance: observed 2026-10-27 UTC in
[PR #140's Linux run](https://github.com/anoop2811/software-factory-template/actions/runs/40447328159),
which recorded a false initial foreground observation and failed resumed input.
That report did not retain the stop signal. The separate real start-boundary
regression held the parent callback until the child reported its first actual tty
group: child/group 28343, foreground 28342 (the supervisor). It failed before the
pre-exec correction; final local native race qualification passed 37 criteria.
