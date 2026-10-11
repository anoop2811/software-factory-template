package native

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
)

func processStopped(ctx context.Context, pid int) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	var information unix.Siginfo
	// Consume only fresh stop events. Cmd.Wait remains the sole exit/reap owner.
	// docs/adr/0098-whole-installation-upgrade-and-rollback.md:581.
	err := unix.Waitid(unix.P_PID, pid, &information, unix.WSTOPPED|unix.WNOHANG, nil)
	if errors.Is(err, unix.ECHILD) || errors.Is(err, unix.ESRCH) {
		return false, ctx.Err()
	}
	if err != nil {
		return false, errors.Join(err, ctx.Err())
	}
	return information.Code != 0, ctx.Err()
}
