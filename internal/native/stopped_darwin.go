package native

import (
	"context"
	"golang.org/x/sys/unix"
)

func processStopped(ctx context.Context, pid int) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	information, err := unix.SysctlKinfoProcSlice("kern.proc.pid", pid)
	if err != nil {
		return false, err
	}
	for _, process := range information {
		// Apple's exported proc.h defines SSTOP=4; fetched 2026-10-09.
		// https://github.com/apple-oss-distributions/xnu/blob/main/bsd/sys/proc.h
		if int64(process.Proc.P_pid) == int64(pid) {
			return process.Proc.P_stat == 4, nil
		}
	}
	return false, ctx.Err()
}
