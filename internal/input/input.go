// Package input adapts command-owned streams for cooperative cancellation.
package input

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/anoop2811/software-factory-template/internal/streamfile"
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
			cleanup := streamfile.CloseOnCancel(ctx, closer)
			return source, cleanup, nil
		}
		return source, func() error { return nil }, nil
	}
	info, err := file.Stat()
	if err != nil {
		return nil, nil, inputError()
	}
	if info.Mode().IsRegular() {
		return source, func() error { return nil }, nil
	}
	// Refuse unqualified devices before changing or closing caller descriptors.
	// docs/adr/0074-go-loop-checkpoint-storage.md:175.
	if info.Mode()&(os.ModeNamedPipe|os.ModeSocket) == 0 {
		return nil, nil, inputError()
	}
	return streamfile.Borrow(ctx, file, ops.newFile)
}
