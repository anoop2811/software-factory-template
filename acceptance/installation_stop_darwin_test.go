package acceptance_test

import (
	"errors"

	"golang.org/x/sys/unix"
)

// Apple's exported stopped state is also used by the native supervisor.
// per docs/DECISION_LOG.md:4695.
func installationProcessStopped(pid int) (bool, error) {
	processes, err := unix.SysctlKinfoProcSlice("kern.proc.pid", pid)
	if errors.Is(err, unix.ESRCH) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, process := range processes {
		if int64(process.Proc.P_pid) == int64(pid) {
			return process.Proc.P_stat == 4, nil
		}
	}
	return false, nil
}
