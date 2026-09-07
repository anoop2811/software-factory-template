package staging

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
)

func snapshot(ctx context.Context, root *os.Root, source, name string, limit int64) (string, error) {
	before, err := os.Lstat(source)
	if err != nil {
		return "", err
	}
	if !before.Mode().IsRegular() || before.Size() > limit {
		return "", errors.New("input must be a bounded regular non-symlink file")
	}
	// O_NONBLOCK prevents a swapped FIFO from hanging before the descriptor can
	// be checked; O_NOFOLLOW rejects a swapped final symlink. Explicit paths are
	// deliberately not cleaned. docs/adr/0061-runtime-bundle-staging.md:27.
	// #nosec G304 -- Caller-selected input, checked without following final symlinks and copied into a bounded private snapshot.
	input, err := os.OpenFile(source, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "", err
	}
	defer input.Close()
	after, err := input.Stat()
	if err != nil {
		return "", err
	}
	if !after.Mode().IsRegular() || !os.SameFile(before, after) || after.Size() > limit {
		return "", errors.New("input changed identity or file type while opening")
	}
	output, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(output, hash), &contextReader{ctx, io.LimitReader(input, limit+1)})
	err = errors.Join(copyErr, output.Close())
	if err != nil {
		return "", err
	}
	if n > limit {
		return "", fmt.Errorf("input exceeds %d bytes", limit)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
