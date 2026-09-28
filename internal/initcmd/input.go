package initcmd

import (
	"context"
	"errors"
	"io"
	"math"
	"os"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/input"
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
		fd, err := unix.Open("/dev/tty", unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if err == nil {
			return &terminalInput{ctx: ctx, fd: fd}, func() error { return unix.Close(fd) }, true, nil
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
			return &terminalInput{ctx: ctx, fd: fd}, cleanup, true, nil
		}
	}
	reader, cleanup, err := input.Cancellable(ctx, source)
	return reader, cleanup, false, err
}

type terminalInput struct {
	ctx context.Context
	fd  int
}

func (t *terminalInput) Read(data []byte) (int, error) {
	if t.fd < 0 || t.fd > math.MaxInt32 {
		return 0, errors.New("invalid terminal descriptor")
	}
	for {
		if err := t.ctx.Err(); err != nil {
			return 0, err
		}
		poll := []unix.PollFd{{Fd: int32(t.fd), Events: unix.POLLIN}}
		_, err := unix.Poll(poll, 50)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return 0, err
		}
		if poll[0].Revents == 0 {
			continue
		}
		n, err := unix.Read(t.fd, data)
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EINTR) {
			continue
		}
		if n == 0 && err == nil {
			return 0, io.EOF
		}
		return n, err
	}
}
