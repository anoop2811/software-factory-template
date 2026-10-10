package installationfs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"os"
)

// File retains a qualified leaf and its confined parent for later use.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:184.
type File struct {
	File   *os.File
	Parent *Node
	Name   string
	stat   unix.Stat_t
	closed bool
}
type Metadata struct {
	Device, Inode                                                uint64
	User, Group, Mode                                            uint32
	Links                                                        uint64
	Size                                                         int64
	ModifiedSeconds, ModifiedNanos, ChangedSeconds, ChangedNanos int64
}

func metadata(stat unix.Stat_t) Metadata {
	device := uint64(stat.Dev) // #nosec G115 -- Opaque native device identity; retain its existing signed bit representation.
	return Metadata{device, uint64(stat.Ino), stat.Uid, stat.Gid, uint32(stat.Mode), uint64(stat.Nlink), stat.Size, stat.Mtim.Sec, stat.Mtim.Nsec, stat.Ctim.Sec, stat.Ctim.Nsec}
}
func (file *File) Metadata() Metadata { return metadata(file.stat) }
func named(ctx context.Context, parent *Node, name string) (stat unix.Stat_t, returned error) {
	if err := parent.tree.Check(ctx); err != nil {
		return stat, err
	}
	directory, err := parent.Root.Open(".")
	if err != nil {
		return stat, err
	}
	defer func() { returned = errors.Join(returned, directory.Close()) }()
	err = unix.Fstatat(int(directory.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW)
	return stat, err
}
func (tree *Tree) OpenFile(ctx context.Context, path string, limit int64) (result *File, returned error) {
	parent, name, _, err := tree.Parent(ctx, path, false)
	if err != nil {
		return nil, err
	}
	before, err := named(ctx, parent, name)
	if err != nil {
		return nil, err
	}
	if before.Mode&unix.S_IFMT != unix.S_IFREG || before.Nlink != 1 || int64(before.Uid) != int64(os.Geteuid()) || before.Mode&07022 != 0 || before.Size < 0 || before.Size > limit {
		return nil, fmt.Errorf("%w: unsafe installation leaf", ErrUnsafe)
	}
	file, err := parent.Root.OpenFile(name, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	var opened unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &opened); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	if metadata(before) != metadata(opened) {
		return nil, errors.Join(fmt.Errorf("%w: installation leaf changed while opening", ErrUnsafe), file.Close())
	}
	pinned := &File{File: file, Parent: parent, Name: name, stat: before}
	if err := pinned.Check(ctx); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return pinned, nil
}
func (file *File) Check(ctx context.Context) error {
	if file == nil || file.closed {
		return errors.New("installation leaf handle unavailable")
	}
	if err := file.Parent.tree.Check(ctx); err != nil {
		return err
	}
	var current unix.Stat_t
	if err := unix.Fstat(int(file.File.Fd()), &current); err != nil {
		return err
	}
	location, err := named(ctx, file.Parent, file.Name)
	if err != nil {
		return err
	}
	if metadata(current) != metadata(file.stat) || metadata(location) != metadata(file.stat) {
		return fmt.Errorf("%w: retained installation leaf changed", ErrUnsafe)
	}
	return ctx.Err()
}
func (file *File) Digest(ctx context.Context, limit int64) (digest string, data []byte, returned error) {
	if err := file.Check(ctx); err != nil {
		return "", nil, err
	}
	if _, err := file.File.Seek(0, io.SeekStart); err != nil {
		return "", nil, err
	}
	hash := sha256.New()
	var count int64
	var buffer [32 << 10]byte
	for {
		if err := ctx.Err(); err != nil {
			return "", nil, err
		}
		n, err := file.File.Read(buffer[:])
		count += int64(n)
		if count > limit {
			return "", nil, fmt.Errorf("%w: installation leaf exceeds limit", ErrUnsafe)
		}
		_, _ = hash.Write(buffer[:n])
		if limit <= 2<<20 {
			data = append(data, buffer[:n]...)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", nil, err
		}
		if n == 0 {
			return "", nil, io.ErrNoProgress
		}
	}
	if err := file.Check(ctx); err != nil {
		return "", nil, err
	}
	return hex.EncodeToString(hash.Sum(nil)), data, nil
}
func (file *File) Rewind(ctx context.Context) error {
	if err := file.Check(ctx); err != nil {
		return err
	}
	_, err := file.File.Seek(0, io.SeekStart)
	return err
}
func (file *File) Close() error {
	if file == nil || file.closed {
		return errors.New("installation leaf handle is closed")
	}
	file.closed = true
	return file.File.Close()
}
