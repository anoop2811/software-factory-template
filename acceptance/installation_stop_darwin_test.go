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

// Darwin reports an exited, unreaped child as SZOMB (5); querying it does not
// consume wait status or release the child's PID.
// per docs/adr/0151-reliable-ci-qualification.md:193
func installationProcessExited(pid int) (bool, error) {
	processes, err := unix.SysctlKinfoProcSlice("kern.proc.pid", pid)
	if err != nil {
		return false, err
	}
	for _, process := range processes {
		if int64(process.Proc.P_pid) == int64(pid) {
			return process.Proc.P_stat == 5, nil
		}
	}
	return false, unix.ESRCH
}
