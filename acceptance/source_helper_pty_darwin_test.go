//go:build darwin

package acceptance_test

import (
	"bytes"
	"os"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Darwin grants the private PTY and returns its slave in a fixed ioctl buffer.
// per docs/adr/0137-go-source-shared-helpers.md:60
func sourceHelperPTYSlave(master *os.File) (string, error) {
	for _, request := range []uint{unix.TIOCPTYGRANT, unix.TIOCPTYUNLK} {
		if err := unix.IoctlSetInt(int(master.Fd()), request, 0); err != nil {
			return "", err
		}
	}
	var name [128]byte
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), unix.TIOCPTYGNAME, uintptr(unsafe.Pointer(&name[0])))
	runtime.KeepAlive(master)
	if errno != 0 {
		return "", errno
	}
	return string(bytes.TrimRight(name[:], "\x00")), nil
}
