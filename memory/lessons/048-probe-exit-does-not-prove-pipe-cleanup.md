# Probe exit status does not prove pipe cleanup

A Git discovery probe can exit nonzero while a descendant retains an inherited
output pipe. Go's WaitDelay bounds waiting, but ErrWaitDelay is not guaranteed to
replace the leader's nonzero exit error. Treating every other error as an ordinary
Git fallback can therefore hide unfinished process cleanup.

Observed 2026-09-26 via the independent `nonzero Git leader` acceptance regression:
the initial command-environment candidate returned success after the child PID
handshake and leader failure. The regression failed before the correction; see
[qualification evidence](../../docs/migration/COMMAND_ENVIRONMENT.md).

`rtk proxy go doc os/exec.Cmd.WaitDelay` on 2026-09-26 confirmed that ErrWaitDelay is
returned for pipe closure when the command otherwise exited successfully and no
Cancel call occurred. Review both process ownership and pipe completion before
classifying a probe failure. The candidate's governing contract remains
[ADR-0077](../../docs/adr/0077-go-command-environment.md); this lesson adds no new
cleanup or activation policy.
