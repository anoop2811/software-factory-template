// Package fileinput reads bounded regular files through validated descriptors.
package fileinput

import (
	"context"
	"errors"
	"io"
	"os"
	"syscall"
)

// Read follows regular-file symlinks but refuses special descriptors before
// reading. NONBLOCK also prevents a FIFO replacement from blocking the open.
// docs/adr/0087-go-native-report.md:58.
func Read(ctx context.Context, path string, limit int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer func() { _ = f.Close() }()
	return ReadFile(ctx, f, limit)
}

// ReadFile leaves ownership of an already-open descriptor with the caller.
func ReadFile(ctx context.Context, f *os.File, limit int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	before, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if limit <= 0 || !before.Mode().IsRegular() || before.Size() > limit {
		return nil, errors.New("unsafe file or input limit exceeded")
	}
	data, err := io.ReadAll(io.LimitReader(contextReader{ctx, f}, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("input limit exceeded")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || before.Mode() != after.Mode() {
		return nil, errors.New("input changed during inspection")
	}
	return data, nil
}

type contextReader struct {
	ctx    context.Context
	source io.Reader
}

func (r contextReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.source.Read(data)
}
