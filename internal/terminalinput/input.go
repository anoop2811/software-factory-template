// Package terminalinput polls owned terminal descriptors for cancellation.
package terminalinput

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"io"
	"math"
)

// Open opens only the controlling terminal, never redirected stdin.
// docs/adr/0089-go-native-review-lane.md:44.
func Open(ctx context.Context) (*Reader, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	fd, err := unix.Open("/dev/tty", unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return Own(ctx, fd), nil
}

// Own takes ownership of an already nonblocking terminal descriptor.
func Own(ctx context.Context, fd int) *Reader { return &Reader{ctx: ctx, fd: fd} }
func (t *Reader) Close() error                { return unix.Close(t.fd) }

type Reader struct {
	ctx context.Context
	fd  int
}

func (t *Reader) Read(data []byte) (int, error) {
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
