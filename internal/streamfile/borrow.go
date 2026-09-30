// Package streamfile borrows pollable descriptors without owning caller streams.
package streamfile

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

func streamError() error { return errors.New("cannot prepare cancellable command stream") }

// Borrow duplicates a pipe/socket descriptor and restores its shared flags at cleanup.
// The caller must not use or alter the original descriptor until cleanup returns.
// newFile may be nil for os.NewFile; input qualification injects setup failures here.
// docs/adr/0088-go-native-metrics.md:86.
func Borrow(ctx context.Context, file *os.File, newFile func(uintptr, string) *os.File) (*os.File, func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil || info.Mode()&(os.ModeNamedPipe|os.ModeSocket) == 0 {
		return nil, nil, streamError()
	}
	raw, err := file.SyscallConn()
	if err != nil {
		return nil, nil, streamError()
	}
	descriptor, flags := -1, 0
	var setupErr error
	err = raw.Control(func(fd uintptr) {
		flags, setupErr = unix.FcntlInt(fd, unix.F_GETFL, 0)
		if setupErr != nil {
			return
		}
		descriptor, setupErr = unix.Dup(int(fd))
		if setupErr != nil {
			return
		}
		unix.CloseOnExec(descriptor)
		setupErr = unix.SetNonblock(descriptor, true)
	})
	restore := func() error {
		var failure error
		err := raw.Control(func(fd uintptr) { _, failure = unix.FcntlInt(fd, unix.F_SETFL, flags) })
		if err != nil || failure != nil {
			return streamError()
		}
		return nil
	}
	if err != nil || setupErr != nil {
		if descriptor >= 0 {
			_ = unix.Close(descriptor)
			if err := restore(); err != nil {
				return nil, nil, err
			}
		}
		return nil, nil, streamError()
	}
	if newFile == nil {
		newFile = os.NewFile
	}
	owned := newFile(uintptr(descriptor), "command-stream")
	if owned == nil {
		_ = unix.Close(descriptor)
		if err := restore(); err != nil {
			return nil, nil, err
		}
		return nil, nil, streamError()
	}
	if err := owned.SetDeadline(time.Time{}); err != nil {
		_ = owned.Close()
		if err := restore(); err != nil {
			return nil, nil, err
		}
		return nil, nil, streamError()
	}
	closeInput := CloseOnCancel(ctx, owned)
	var once sync.Once
	var cleanupErr error
	cleanup := func() error {
		once.Do(func() {
			closeErr := closeInput()
			restoreErr := restore()
			if closeErr != nil || restoreErr != nil {
				cleanupErr = streamError()
			}
		})
		return cleanupErr
	}
	if err := ctx.Err(); err != nil {
		_ = cleanup()
		return nil, nil, err
	}
	return owned, cleanup, nil
}

// CloseOnCancel closes an owned stream on cancellation and joins that callback at cleanup.
func CloseOnCancel(ctx context.Context, closer io.Closer) func() error {
	done := make(chan struct{})
	var closeErr error
	stop := context.AfterFunc(ctx, func() { closeErr = closer.Close(); close(done) })
	var once sync.Once
	var result error
	return func() error {
		once.Do(func() {
			if stop() {
				closeErr = closer.Close()
			} else {
				<-done
			}
			if closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
				result = streamError()
			}
		})
		return result
	}
}
