//go:build linux

package native_test

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:634
func nativePTYSlave(master *os.File) (string, error) {
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		return "", err
	}
	number, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("/dev/pts/%d", number), nil
}
