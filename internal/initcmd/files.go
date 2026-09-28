package initcmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const fileLimit = 16 << 20

type tree struct {
	path string
	root *os.File
}

func physicalPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	current := absolute
	var tail []string
	for {
		_, err := os.Lstat(current)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		tail = append(tail, filepath.Base(current))
		current = filepath.Dir(current)
	}
	current, err = filepath.EvalSymlinks(current)
	if err != nil {
		return "", err
	}
	for i := len(tail) - 1; i >= 0; i-- {
		current = filepath.Join(current, tail[i])
	}
	return current, nil
}

func openTree(path string) (*tree, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), "init-root")
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || !safeDirectory(stat) {
		_ = file.Close()
		return nil, errors.New("unsafe installation directory")
	}
	return &tree{path: path, root: file}, nil
}
func safeDirectory(stat unix.Stat_t) bool {
	return stat.Mode&unix.S_IFMT == unix.S_IFDIR && stat.Mode&07022 == 0 && int64(stat.Uid) == int64(os.Geteuid())
}
func safeFile(stat unix.Stat_t) bool {
	return stat.Mode&unix.S_IFMT == unix.S_IFREG && stat.Mode&07000 == 0 && stat.Nlink == 1 && stat.Size >= 0 && stat.Size <= fileLimit && int64(stat.Uid) == int64(os.Geteuid())
}
func fileStat(file *os.File) (unix.Stat_t, error) {
	var s unix.Stat_t
	err := unix.Fstat(int(file.Fd()), &s)
	return s, err
}
func sameFile(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode && a.Nlink == b.Nlink && a.Size == b.Size && a.Mtim == b.Mtim && a.Ctim == b.Ctim && a.Uid == b.Uid
}

// Every planned descendant is opened relative to a pinned directory without links.
// docs/adr/0085-go-native-init.md:73.
func (t *tree) parent(ctx context.Context, relative string, create bool) (*os.File, string, error) {
	if filepath.IsAbs(relative) || strings.ContainsRune(relative, 0) {
		return nil, "", errors.New("invalid installation path")
	}
	parts := strings.Split(filepath.ToSlash(relative), "/")
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return nil, "", errors.New("invalid installation path")
		}
	}
	fd, err := unix.Dup(int(t.root.Fd()))
	if err != nil {
		return nil, "", err
	}
	unix.CloseOnExec(fd)
	current := os.NewFile(uintptr(fd), "init-directory")
	for _, part := range parts[:len(parts)-1] {
		if err := ctx.Err(); err != nil {
			_ = current.Close()
			return nil, "", err
		}
		next, err := unix.Openat(int(current.Fd()), part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if errors.Is(err, unix.ENOENT) && create {
			if err = unix.Mkdirat(int(current.Fd()), part, 0755); err == nil || errors.Is(err, unix.EEXIST) {
				next, err = unix.Openat(int(current.Fd()), part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			}
		}
		_ = current.Close()
		if err != nil {
			return nil, "", err
		}
		current = os.NewFile(uintptr(next), "init-directory")
		stat, err := fileStat(current)
		if err != nil || !safeDirectory(stat) {
			_ = current.Close()
			return nil, "", errors.New("unsafe installation directory")
		}
	}
	return current, parts[len(parts)-1], nil
}
func (t *tree) read(ctx context.Context, relative string) ([]byte, os.FileMode, bool, error) {
	data, mode, exists, _, err := t.observe(ctx, relative)
	return data, mode, exists, err
}
func (t *tree) observe(ctx context.Context, relative string) ([]byte, os.FileMode, bool, unix.Stat_t, error) {
	parent, name, err := t.parent(ctx, relative, false)
	if errors.Is(err, unix.ENOENT) {
		return nil, 0, false, unix.Stat_t{}, nil
	}
	if err != nil {
		return nil, 0, false, unix.Stat_t{}, err
	}
	defer parent.Close()
	fd, err := unix.Openat(int(parent.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil, 0, false, unix.Stat_t{}, nil
	}
	if err != nil {
		return nil, 0, false, unix.Stat_t{}, err
	}
	file := os.NewFile(uintptr(fd), "init-input")
	defer file.Close()
	before, err := fileStat(file)
	if err != nil || !safeFile(before) {
		return nil, 0, false, unix.Stat_t{}, errors.New("unsafe installation file")
	}
	if err := ctx.Err(); err != nil {
		return nil, 0, false, unix.Stat_t{}, err
	}
	data, err := io.ReadAll(io.LimitReader(file, fileLimit+1))
	if err != nil {
		return nil, 0, false, unix.Stat_t{}, err
	}
	if err := ctx.Err(); err != nil {
		return nil, 0, false, unix.Stat_t{}, err
	}
	after, err := fileStat(file)
	if err != nil || len(data) > fileLimit || !sameFile(before, after) {
		return nil, 0, false, unix.Stat_t{}, errors.New("installation file changed")
	}
	var location unix.Stat_t
	if err := unix.Fstatat(int(parent.Fd()), name, &location, unix.AT_SYMLINK_NOFOLLOW); err != nil || !sameFile(after, location) {
		return nil, 0, false, unix.Stat_t{}, errors.New("installation file changed")
	}
	return data, os.FileMode(before.Mode & 0777), true, before, nil
}
func (t *tree) list(ctx context.Context, relative string) ([]os.DirEntry, error) {
	parent, name, err := t.parent(ctx, relative, false)
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	fd, err := unix.Openat(int(parent.Fd()), name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	dir := os.NewFile(uintptr(fd), "init-assets")
	defer dir.Close()
	stat, err := fileStat(dir)
	if err != nil || !safeDirectory(stat) {
		return nil, errors.New("unsafe source directory")
	}
	entries, err := dir.ReadDir(4097)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > 4096 {
		return nil, errors.New("too many source assets")
	}
	return entries, ctx.Err()
}

type asset struct {
	path             string
	data             []byte
	mode             os.FileMode
	previous         []byte
	previousMode     os.FileMode
	previousIdentity unix.Stat_t
	existed          bool
}

func (t *tree) publish(ctx context.Context, a asset) (returned error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	parent, name, err := t.parent(ctx, a.path, true)
	if err != nil {
		return err
	}
	defer parent.Close()
	data, mode, exists, original, err := t.observe(ctx, a.path)
	if err != nil {
		return err
	}
	if exists != a.existed || string(data) != string(a.previous) || mode != a.previousMode || exists && !sameFile(original, a.previousIdentity) {
		return errors.New("installation destination changed")
	}
	if exists && string(data) == string(a.data) && mode == a.mode {
		return nil
	}
	if exists {
		backup := name + ".factory-backup." + time.Now().Format("20060102150405")
		preserved := false
		for n := 0; n < 128; n++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			candidate := backup
			if n > 0 {
				candidate += "." + strconv.Itoa(n)
			}
			fd, err := unix.Openat(int(parent.Fd()), candidate, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
			if errors.Is(err, unix.EEXIST) {
				continue
			}
			if err != nil {
				return err
			}
			file := os.NewFile(uintptr(fd), "init-backup")
			_, writeErr := file.Write(data)
			syncErr := file.Sync()
			closeErr := file.Close()
			if writeErr != nil || syncErr != nil || closeErr != nil {
				return errors.New("cannot preserve installation file")
			}
			preserved = true
			break
		}
		if !preserved {
			return errors.New("cannot reserve installation backup")
		}
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return err
	}
	temporary := ".factory-init-" + hex.EncodeToString(random[:])
	fd, err := unix.Openat(int(parent.Fd()), temporary, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), "init-output")
	defer func() { returned = errors.Join(returned, file.Close()) }()
	created, err := fileStat(file)
	if err != nil {
		return err
	}
	published := false
	defer func() {
		if published {
			return
		}
		var occupant unix.Stat_t
		err := unix.Fstatat(int(parent.Fd()), temporary, &occupant, unix.AT_SYMLINK_NOFOLLOW)
		switch {
		case errors.Is(err, unix.ENOENT):
		case err != nil:
			returned = errors.Join(returned, errors.New("cannot inspect initializer temporary file"))
		case created.Dev != occupant.Dev || created.Ino != occupant.Ino || created.Mode&unix.S_IFMT != occupant.Mode&unix.S_IFMT:
			returned = errors.Join(returned, errors.New("initializer temporary identity changed; cleanup refused"))
		default:
			returned = errors.Join(returned, unix.Unlinkat(int(parent.Fd()), temporary, 0))
		}
	}()
	if _, err := file.Write(a.data); err != nil {
		return err
	}
	if err := file.Chmod(a.mode); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	prepared, err := fileStat(file)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var current unix.Stat_t
	statErr := unix.Fstatat(int(parent.Fd()), name, &current, unix.AT_SYMLINK_NOFOLLOW)
	if exists {
		if statErr != nil || !sameFile(original, current) {
			return errors.New("installation destination changed")
		}
	} else if !errors.Is(statErr, unix.ENOENT) {
		return errors.New("installation destination changed")
	}
	currentTemporary, err := fileStat(file)
	if err != nil || !sameFile(prepared, currentTemporary) {
		return errors.New("initializer temporary file changed")
	}
	var namedTemporary unix.Stat_t
	if err := unix.Fstatat(int(parent.Fd()), temporary, &namedTemporary, unix.AT_SYMLINK_NOFOLLOW); err != nil || !sameFile(prepared, namedTemporary) {
		return errors.New("initializer temporary file changed")
	}
	if err := unix.Renameat(int(parent.Fd()), temporary, int(parent.Fd()), name); err != nil {
		return err
	}
	published = true
	return parent.Sync()
}
