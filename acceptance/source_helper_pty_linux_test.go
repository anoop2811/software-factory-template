//go:build linux

package acceptance_test

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// Linux exposes the private PTY slave number after unlocking its master.
// per docs/adr/0137-go-source-shared-helpers.md:60
func sourceHelperPTYSlave(master *os.File) (string, error) {
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		return "", err
	}
	number, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("/dev/pts/%d", number), nil
}
