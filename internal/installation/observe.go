package installation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func identity(stat unix.Stat_t) Identity {
	device := uint64(stat.Dev) // #nosec G115 -- Opaque native device identity; retain its existing signed bit representation.
	return Identity{Device: device, Inode: uint64(stat.Ino), User: stat.Uid, Group: stat.Gid, Mode: uint32(stat.Mode), Links: uint64(stat.Nlink), Size: stat.Size, ModifiedSeconds: stat.Mtim.Sec, ModifiedNanos: stat.Mtim.Nsec, ChangedSeconds: stat.Ctim.Sec, ChangedNanos: stat.Ctim.Nsec}
}
func safeDirectory(stat unix.Stat_t) bool {
	return stat.Mode&unix.S_IFMT == unix.S_IFDIR && stat.Mode&07022 == 0 && int64(stat.Uid) == int64(os.Geteuid())
}
func statFile(file *os.File) (unix.Stat_t, error) {
	var stat unix.Stat_t
	err := unix.Fstat(int(file.Fd()), &stat)
	return stat, err
}

type observeOps struct {
	statFile func(*os.File) (unix.Stat_t, error)
}

func observe(ctx context.Context, tree *installationfs.Tree, path string, limit int64) (*Image, []byte, error) {
	return observeWith(ctx, tree, path, limit, observeOps{})
}

func observeWith(ctx context.Context, tree *installationfs.Tree, path string, limit int64, operations observeOps) (result *Image, saved []byte, returned error) {
	observeStat := operations.statFile
	if observeStat == nil {
		observeStat = statFile
	}
	if path == "" || filepath.IsAbs(path) || strings.ContainsAny(path, "\\\x00") {
		return nil, nil, conflict("unsafe installation selection")
	}
	parent, name, _, err := tree.Parent(ctx, path, false)
	if errors.Is(err, os.ErrNotExist) && !errors.Is(err, installationfs.ErrUnsafe) {
		return nil, nil, nil
	}
	if err != nil {
		if errors.Is(err, installationfs.ErrUnsafe) {
			return nil, nil, &Failure{Code: 2, Reason: "unsafe installation parent", cause: err}
		}
		return nil, nil, err
	}
	root := parent.Root
	path = name
	info, err := root.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > limit || info.Mode()&os.ModeType != 0 {
		return nil, nil, conflict("unsafe or oversized installation file")
	}
	file, err := root.OpenFile(path, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, err
	}
	defer func() { returned = errors.Join(returned, file.Close()) }()
	before, err := observeStat(file)
	if err != nil {
		return nil, nil, err
	}
	openedInfo, infoErr := file.Stat()
	if infoErr != nil {
		return nil, nil, infoErr
	}
	if before.Nlink != 1 || int64(before.Uid) != int64(os.Geteuid()) || before.Mode&07022 != 0 || !os.SameFile(info, openedInfo) {
		return nil, nil, conflict("unsafe installation file identity")
	}
	hash := sha256.New()
	var data []byte
	var buffer [32 << 10]byte
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		n, readErr := file.Read(buffer[:])
		total += int64(n)
		if total > limit {
			return nil, nil, conflict("installation file exceeds limit")
		}
		_, _ = hash.Write(buffer[:n])
		if limit <= 2<<20 {
			data = append(data, buffer[:n]...)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, nil, readErr
		}
		if n == 0 {
			return nil, nil, io.ErrNoProgress
		}
	}
	after, err := observeStat(file)
	if err != nil {
		return nil, nil, err
	}
	if identity(before) != identity(after) {
		return nil, nil, conflict("installation file changed during observation")
	}
	if err := tree.Check(ctx); err != nil {
		return nil, nil, &Failure{Code: 2, Reason: "installation parent changed during observation", cause: err}
	}
	named, err := root.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if !os.SameFile(info, named) || named.Mode() != info.Mode() || named.Size() != info.Size() || !named.ModTime().Equal(info.ModTime()) {
		return nil, nil, conflict("installation file changed during observation")
	}
	id := identity(before)
	return &Image{Type: "regular", Mode: uint32(before.Mode), SHA256: hex.EncodeToString(hash.Sum(nil)), Bytes: total, Identity: &id}, data, nil
}
func bytesImage(data []byte, mode uint32) *Image {
	sum := sha256.Sum256(data)
	return &Image{Type: "regular", Mode: unix.S_IFREG | mode, SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(data))}
}
