// Package filepublish prepares exclusive siblings for caller-validated publication.
package filepublish

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"syscall"
)

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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	name := prefix + rand.Text()
	file, err := directory.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	identity, err := file.Stat()
	if err != nil {
		return nil, errors.Join(err, file.Close())
	}
	owned := &Stage{directory: directory, name: name, identity: identity}
	closed := false
	defer func() {
		if !closed {
			returned = errors.Join(returned, file.Close())
		}
		if returned != nil {
			returned = errors.Join(returned, owned.Cleanup())
		}
	}()
	if n, err := file.Write(data); err != nil {
		return nil, err
	} else if n != len(data) {
		return nil, io.ErrShortWrite
	}
	if err := file.Chmod(mode); err != nil {
		return nil, err
	}
	if err := file.Sync(); err != nil {
		return nil, err
	}
	prepared, err := file.Stat()
	if err != nil {
		return nil, err
	}
	owned.identity = prepared
	err = file.Close()
	closed = true
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return owned, nil
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
