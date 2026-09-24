// Package input adapts command-owned streams for cooperative cancellation.
package input

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

type setupOps struct {
	newFile func(uintptr, string) *os.File
}

func inputError() error { return errors.New("cannot prepare cancellable command input") }

// Cancellable returns an input view and mandatory cleanup. Pipe/socket descriptors
// are duplicated without taking ownership of the caller's original descriptor.
// Callers must not read from or change the original stream until cleanup returns.
// Regular filesystem reads retain their existing syscall deadline limitations.
// docs/adr/0074-go-loop-checkpoint-storage.md:134.
func Cancellable(ctx context.Context, source io.Reader) (io.Reader, func() error, error) {
	return prepare(ctx, source, setupOps{})
}
func prepare(ctx context.Context, source io.Reader, ops setupOps) (io.Reader, func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	file, ok := source.(*os.File)
	if !ok {
		if closer, ok := source.(io.ReadCloser); ok {
			cleanup := cancelClose(ctx, closer)
			return source, cleanup, nil
		}
		return source, func() error { return nil }, nil
	}
	info, err := file.Stat()
	if err != nil {
		return nil, nil, inputError()
	}
	if info.Mode()&(os.ModeNamedPipe|os.ModeSocket) == 0 {
		return source, func() error { return nil }, nil
	}
	raw, err := file.SyscallConn()
	if err != nil {
		return nil, nil, inputError()
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
			return inputError()
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
		return nil, nil, inputError()
	}
	newFile := ops.newFile
	if newFile == nil {
		newFile = os.NewFile
	}
	owned := newFile(uintptr(descriptor), "command-input")
	if owned == nil {
		_ = unix.Close(descriptor)
		if err := restore(); err != nil {
			return nil, nil, err
		}
		return nil, nil, inputError()
	}
	if err := owned.SetReadDeadline(time.Time{}); err != nil {
		_ = owned.Close()
		if err := restore(); err != nil {
			return nil, nil, err
		}
		return nil, nil, inputError()
	}
	closeInput := cancelClose(ctx, owned)
	var once sync.Once
	var cleanupErr error
	cleanup := func() error {
		once.Do(func() {
			closeErr := closeInput()
			restoreErr := restore()
			if closeErr != nil || restoreErr != nil {
				cleanupErr = inputError()
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
func cancelClose(ctx context.Context, closer io.Closer) func() error {
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
				result = inputError()
			}
		})
		return result
	}
}
