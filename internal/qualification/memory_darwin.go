package qualification

import "golang.org/x/sys/unix"

func platformMemory() (uint64, error) { return unix.SysctlUint64("hw.memsize") }
