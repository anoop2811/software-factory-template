package initcmd

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/input"
	"github.com/anoop2811/software-factory-template/internal/terminalinput"
	"golang.org/x/sys/unix"
)

func terminal(file *os.File) bool {
	_, err := unix.IoctlGetWinsize(int(file.Fd()), unix.TIOCGWINSZ)
	return err == nil
}

// Terminal reads are polled on owned nonblocking descriptors, never a worker
// goroutine that can outlive cancellation. docs/adr/0085-go-native-init.md:39.
func promptInput(ctx context.Context, source io.Reader, out io.Writer) (io.Reader, func() error, bool, error) {
	if file, ok := out.(*os.File); ok && terminal(file) {
		reader, err := terminalinput.Open(ctx)
		if err == nil {
			return reader, reader.Close, true, nil
		}
	}
	if file, ok := source.(*os.File); ok {
		stat, err := file.Stat()
		if err != nil {
			return nil, nil, false, err
		}
		if null, err := os.Stat("/dev/null"); err == nil && os.SameFile(stat, null) {
			return strings.NewReader(""), func() error { return nil }, false, nil
		}
		if terminal(file) {
			flags, err := unix.FcntlInt(file.Fd(), unix.F_GETFL, 0)
			if err != nil {
				return nil, nil, false, err
			}
			fd, err := unix.Dup(int(file.Fd()))
			if err != nil {
				return nil, nil, false, err
			}
			unix.CloseOnExec(fd)
			if err := unix.SetNonblock(fd, true); err != nil {
				_ = unix.Close(fd)
				return nil, nil, false, err
			}
			cleanup := func() error {
				_, restore := unix.FcntlInt(file.Fd(), unix.F_SETFL, flags)
				return errors.Join(unix.Close(fd), restore)
			}
			return terminalinput.Own(ctx, fd), cleanup, true, nil
		}
	}
	reader, cleanup, err := input.Cancellable(ctx, source)
	return reader, cleanup, false, err
}
