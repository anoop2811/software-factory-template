// Package filepublish prepares exclusive siblings for caller-validated publication.
package filepublish

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"syscall"
)

type ops struct {
	write    func(context.Context, *os.File, []byte) (int, error)
	read     func(context.Context, *os.File, []byte) (int, error)
	syncFile func(context.Context, *os.File) error
	close    func(*os.File) error
}

// Stage owns only its exclusive temporary name, never the caller's directory.
// Destination ownership is the caller's policy. docs/adr/0089-go-native-review-lane.md:72.
type Stage struct {
	directory *os.Root
	name      string
	identity  os.FileInfo
	published bool
}

func samePrepared(a, b os.FileInfo) bool {
	x, xok := a.Sys().(*syscall.Stat_t)
	y, yok := b.Sys().(*syscall.Stat_t)
	return xok && yok && b.Mode().IsRegular() && b.Mode() == b.Mode().Perm() &&
		x.Nlink == 1 && y.Nlink == 1 && x.Uid == y.Uid && x.Gid == y.Gid &&
		os.SameFile(a, b) && a.Mode() == b.Mode() && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}

func Prepare(ctx context.Context, directory *os.Root, prefix string, data []byte, mode os.FileMode) (stage *Stage, returned error) {
	return prepare(ctx, directory, prefix, data, mode, ops{})
}

func prepare(ctx context.Context, directory *os.Root, prefix string, data []byte, mode os.FileMode, operations ops) (stage *Stage, returned error) {
	return prepareOperation(ctx, directory, prefix, mode, operations,
		func(ctx context.Context, file *os.File) error {
			write := operations.write
			if write == nil {
				write = func(_ context.Context, file *os.File, data []byte) (int, error) { return file.Write(data) }
			}
			n, err := write(ctx, file, data)
			if err != nil {
				return err
			}
			if n != len(data) {
				return io.ErrShortWrite
			}
			return nil
		}, func(ctx context.Context, file *os.File) error { return verifyPrepared(ctx, file, data, operations) })
}
func prepareOperation(ctx context.Context, directory *os.Root, prefix string, mode os.FileMode, operations ops, writeSource, verifySource func(context.Context, *os.File) error) (stage *Stage, returned error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	name := prefix + rand.Text()
	file, err := directory.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return nil, err
	}
	closeFile := operations.close
	if closeFile == nil {
		closeFile = (*os.File).Close
	}
	identity, err := file.Stat()
	if err != nil {
		return nil, errors.Join(err, closeFile(file))
	}
	owned := &Stage{directory: directory, name: name, identity: identity}
	closed := false
	defer func() {
		if !closed {
			returned = errors.Join(returned, closeFile(file))
		}
		if returned != nil {
			returned = errors.Join(returned, owned.Cleanup())
		}
	}()
	if err := writeSource(ctx, file); err != nil {
		return nil, err
	}
	if err := file.Chmod(mode); err != nil {
		return nil, err
	}
	syncFile := operations.syncFile
	if syncFile == nil {
		syncFile = func(_ context.Context, file *os.File) error { return file.Sync() }
	}
	if err := syncFile(ctx, file); err != nil {
		return nil, err
	}
	// Preparation verifies actual bytes before exposing its inode to a caller.
	// docs/adr/0094-live-publication-restoration.md:72.
	if err := verifySource(ctx, file); err != nil {
		return nil, err
	}
	prepared, err := file.Stat()
	if err != nil {
		return nil, err
	}
	owned.identity = prepared
	err = closeFile(file)
	closed = true
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return owned, nil
}

func verifyPrepared(ctx context.Context, file *os.File, expected []byte, operations ops) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	read := operations.read
	if read == nil {
		read = func(_ context.Context, file *os.File, data []byte) (int, error) { return file.Read(data) }
	}
	var data []byte
	var buffer [32 << 10]byte
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := read(ctx, file, buffer[:])
		if n < 0 || n > len(buffer) || n > len(expected)-len(data) {
			return errors.New("prepared bytes changed before publication")
		}
		data = append(data, buffer[:n]...)
		if err := ctx.Err(); err != nil {
			return err
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrNoProgress
		}
	}
	if !bytes.Equal(data, expected) {
		return errors.New("prepared bytes changed before publication")
	}
	return nil
}

// Name identifies the owned staging entry, not publication authority.
func (s *Stage) Name() string { return s.name }

// OpenPrepared retains a descriptor proved against this stage's actual inode.
// docs/adr/0094-live-publication-restoration.md:75.
func (s *Stage) OpenPrepared(ctx context.Context) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.published {
		return nil, errors.New("staging ownership unavailable")
	}
	current, err := s.directory.Lstat(s.name)
	if err != nil || !samePrepared(s.identity, current) {
		return nil, errors.New("temporary changed before publication")
	}
	file, err := s.directory.OpenFile(s.name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil || !samePrepared(s.identity, opened) {
		return nil, errors.Join(errors.New("temporary changed while opening"), file.Close())
	}
	return file, nil
}

func (s *Stage) Cleanup() error {
	if s.published {
		return nil
	}
	current, err := s.directory.Lstat(s.name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !os.SameFile(s.identity, current) {
		return errors.New("temporary identity changed; cleanup refused")
	}
	return s.directory.Remove(s.name)
}

func (s *Stage) Publish(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	current, err := s.directory.Lstat(s.name)
	if err != nil {
		return err
	}
	if !samePrepared(s.identity, current) {
		return errors.New("temporary changed before publication")
	}
	if err := s.directory.Rename(s.name, name); err != nil {
		return err
	}
	s.published = true
	return nil
}
