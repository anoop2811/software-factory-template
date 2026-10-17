//go:build darwin

package native_test

import (
	"bytes"
	"golang.org/x/sys/unix"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

// Darwin exposes its PTY slave name through a fixed 128-byte ioctl buffer.
// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:634
func nativePTYSlave(master *os.File) (string, error) {
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
