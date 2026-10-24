package acceptance_test

import (
	"errors"

	"golang.org/x/sys/unix"
)

// Each caller owns a fresh child and observes its first requested stop.
// Do not consume any status; Cmd.Wait owns exit and reaping.
// per docs/DECISION_LOG.md:4695.
func installationProcessStopped(pid int) (bool, error) {
	var information unix.Siginfo
	err := unix.Waitid(unix.P_PID, pid, &information, unix.WSTOPPED|unix.WNOHANG|unix.WNOWAIT, nil)
	if errors.Is(err, unix.ECHILD) || errors.Is(err, unix.ESRCH) {
		return false, nil
	}
	return information.Code != 0, err
}
