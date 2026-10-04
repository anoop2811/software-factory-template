package assessment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// Per-operation collaborators keep fault injection scoped to actual I/O.
type recoveryWriteOps struct {
	open          func(context.Context, *os.File, string, int, uint32) (*os.File, error)
	write         func(context.Context, *os.File, []byte) (int, error)
	writeAt       func(context.Context, *os.File, []byte, int64) (int, error)
	read          func(context.Context, *os.File, []byte) (int, error)
	syncFile      func(context.Context, *os.File) error
	syncDirectory func(context.Context, *os.File) error
	close         func(*os.File) error
}

type writePin struct {
	parent *os.File
	file   *os.File
	name   string
	stat   unix.Stat_t
}

type recoveryWriter struct {
	chain   directories
	pins    []*writePin
	root    *writePin
	ops     recoveryWriteOps
	changed bool
}

func trustedDirectory(stat unix.Stat_t, private bool) bool {
	return stat.Mode&unix.S_IFMT == unix.S_IFDIR && int64(stat.Uid) == int64(os.Geteuid()) && stat.Mode&07022 == 0 && (!private || stat.Mode&07777 == 0700)
}

func ownedFile(stat unix.Stat_t, limit int64) bool {
	return stat.Mode&unix.S_IFMT == unix.S_IFREG && stat.Nlink == 1 && stat.Size >= 0 && stat.Size <= limit && stat.Mode&07000 == 0 && int64(stat.Uid) == int64(os.Geteuid())
}

func writeUnchanged(before, after unix.Stat_t) bool {
	return recoveryUnchanged(before, after) && before.Gid == after.Gid
}

func (w *recoveryWriter) close() error {
	var errs []error
	for i := len(w.pins) - 1; i >= 0; i-- {
		if w.pins[i] != w.root {
			errs = append(errs, w.closeFile(w.pins[i].file))
		}
	}
	for i := len(w.chain) - 1; i >= 0; i-- {
		errs = append(errs, w.closeFile(w.chain[i].file))
	}
	if errors.Join(errs...) != nil {
		return failure(1, "cannot close recovery storage")
	}
	return nil
}

func (w *recoveryWriter) closeFile(file *os.File) error {
	if w.ops.close != nil {
		return w.ops.close(file)
	}
	return file.Close()
}

// All named identities and immutable metadata stay pinned. A successful owned
// mutation refreshes only its exact file/parent, never an unexplained ancestor.
// docs/adr/0091-durable-local-recovery-creation.md:98.
func (w *recoveryWriter) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for i, entry := range w.chain {
		current, err := descriptor(entry.file)
		if err != nil || !sameIdentity(current, entry.identity) || current.Mode != entry.identity.Mode || current.Uid != entry.identity.Uid || current.Gid != entry.identity.Gid {
			return failure(2, "recovery installation ancestry changed")
		}
		var location unix.Stat_t
		if i == 0 {
			err = unix.Fstatat(unix.AT_FDCWD, entry.name, &location, unix.AT_SYMLINK_NOFOLLOW)
		} else {
			location, err = named(w.chain[i-1].file, entry.name)
		}
		if err != nil || !sameIdentity(location, entry.identity) || location.Mode != entry.identity.Mode || location.Uid != entry.identity.Uid || location.Gid != entry.identity.Gid {
			return failure(2, "recovery installation ancestry changed")
		}
	}
	for _, pin := range w.pins {
		current, err := descriptor(pin.file)
		if err != nil || !writeUnchanged(pin.stat, current) {
			return failure(2, "recovery storage or selected input changed")
		}
		if pin.parent != nil {
			location, err := named(pin.parent, pin.name)
			if err != nil || !writeUnchanged(pin.stat, location) {
				return failure(2, "recovery storage or selected input changed")
			}
		}
	}
	return nil
}

func (w *recoveryWriter) refresh(pin *writePin) error {
	current, err := descriptor(pin.file)
	if err != nil || !sameIdentity(pin.stat, current) || current.Mode != pin.stat.Mode || current.Uid != pin.stat.Uid || current.Gid != pin.stat.Gid {
		return failure(2, "recovery mutation identity changed")
	}
	if pin.parent != nil {
		location, err := named(pin.parent, pin.name)
		if err != nil || !writeUnchanged(current, location) {
			return failure(2, "recovery mutation identity changed")
		}
	}
	pin.stat = current
	return nil
}

func (w *recoveryWriter) open(ctx context.Context, parent *os.File, name string, flags int, mode uint32) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if w.ops.open != nil {
		return w.ops.open(ctx, parent, name, flags|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, mode)
	}
	fd, err := unix.Openat(int(parent.Fd()), name, flags|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, mode)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}

func (w *recoveryWriter) directory(ctx context.Context, parent *writePin, name string, private, create bool) (*writePin, error) {
	if err := w.check(ctx); err != nil {
		return nil, err
	}
	for _, pin := range w.pins {
		if pin.parent == parent.file && pin.name == name {
			if !trustedDirectory(pin.stat, private) {
				return nil, failure(2, "unsafe recovery directory")
			}
			return pin, nil
		}
	}
	before, err := named(parent.file, name)
	if errors.Is(err, unix.ENOENT) && create {
		if err := unix.Mkdirat(int(parent.file.Fd()), name, 0700); err != nil {
			return nil, creationError(err, "cannot exclusively create recovery directory")
		}
		w.changed = true
		if err := w.refresh(parent); err != nil {
			return nil, err
		}
		before, err = named(parent.file, name)
	}
	if err != nil || !trustedDirectory(before, private) {
		return nil, failure(2, "unsafe or unavailable recovery directory")
	}
	file, err := w.open(ctx, parent.file, name, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		return nil, failure(1, "cannot open recovery directory")
	}
	after, err := descriptor(file)
	if err != nil || !writeUnchanged(before, after) {
		_ = file.Close()
		return nil, failure(2, "recovery directory changed while opening")
	}
	pin := &writePin{parent: parent.file, file: file, name: name, stat: after}
	w.pins = append(w.pins, pin)
	return pin, nil
}

func (w *recoveryWriter) existingFile(ctx context.Context, parent *writePin, name string, flags int, limit int64, private bool) (*writePin, error) {
	if err := w.check(ctx); err != nil {
		return nil, err
	}
	before, err := named(parent.file, name)
	if err != nil || !ownedFile(before, limit) || private && before.Mode&07777 != 0600 {
		return nil, failure(2, "unsafe or unavailable recovery file")
	}
	file, err := w.open(ctx, parent.file, name, flags, 0)
	if err != nil {
		return nil, failure(1, "cannot open recovery file")
	}
	after, err := descriptor(file)
	if err != nil || !writeUnchanged(before, after) {
		_ = file.Close()
		return nil, failure(2, "recovery file changed while opening")
	}
	pin := &writePin{parent: parent.file, file: file, name: name, stat: after}
	w.pins = append(w.pins, pin)
	return pin, nil
}

func (w *recoveryWriter) createFile(ctx context.Context, parent *writePin, name string) (*writePin, error) {
	if err := w.check(ctx); err != nil {
		return nil, err
	}
	file, err := w.open(ctx, parent.file, name, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL, 0600)
	if err != nil {
		return nil, creationError(err, "cannot exclusively reserve recovery file")
	}
	w.changed = true
	after, err := descriptor(file)
	if err != nil || !recoveryFile(after, assetLimit) {
		_ = file.Close()
		return nil, failure(2, "unsafe created recovery file")
	}
	pin := &writePin{parent: parent.file, file: file, name: name, stat: after}
	w.pins = append(w.pins, pin)
	if err := w.refresh(parent); err != nil {
		return nil, err
	}
	return pin, nil
}

func (w *recoveryWriter) write(ctx context.Context, pin *writePin, data []byte) error {
	if err := w.check(ctx); err != nil {
		return err
	}
	var n int
	var err error
	if w.ops.write != nil {
		n, err = w.ops.write(ctx, pin.file, data)
	} else {
		n, err = pin.file.Write(data)
	}
	if err != nil || n != len(data) {
		return failure(1, "cannot completely write recovery data")
	}
	if err := w.refresh(pin); err != nil {
		return err
	}
	return ctx.Err()
}

func (w *recoveryWriter) writeAt(ctx context.Context, pin *writePin, data []byte, offset int64) error {
	if err := w.check(ctx); err != nil {
		return err
	}
	var n int
	var err error
	if w.ops.writeAt != nil {
		n, err = w.ops.writeAt(ctx, pin.file, data, offset)
	} else {
		n, err = pin.file.WriteAt(data, offset)
	}
	if err != nil || n != len(data) {
		return failure(1, "cannot completely activate local recovery exclusion")
	}
	if err := w.refresh(pin); err != nil {
		return err
	}
	return ctx.Err()
}

func (w *recoveryWriter) sync(ctx context.Context, pin *writePin) error {
	if err := w.check(ctx); err != nil {
		return err
	}
	operation := w.ops.syncFile
	if pin.stat.Mode&unix.S_IFMT == unix.S_IFDIR {
		operation = w.ops.syncDirectory
	}
	var err error
	if operation != nil {
		err = operation(ctx, pin.file)
	} else {
		err = pin.file.Sync()
	}
	if err != nil {
		return failure(1, "cannot sync recovery storage")
	}
	return w.check(ctx)
}

func (w *recoveryWriter) read(ctx context.Context, pin *writePin, limit int64) ([]byte, error) {
	if err := w.check(ctx); err != nil {
		return nil, err
	}
	if _, err := pin.file.Seek(0, io.SeekStart); err != nil {
		return nil, failure(1, "cannot read recovery storage")
	}
	var data []byte
	var buffer [32 << 10]byte
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var n int
		var err error
		if w.ops.read != nil {
			n, err = w.ops.read(ctx, pin.file, buffer[:])
		} else {
			n, err = pin.file.Read(buffer[:])
		}
		if n < 0 || n > len(buffer) || int64(len(data))+int64(n) > limit {
			return nil, failure(1, "cannot read bounded recovery storage")
		}
		data = append(data, buffer[:n]...)
		if canceled := ctx.Err(); canceled != nil {
			return nil, canceled
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || n == 0 {
			return nil, failure(1, "cannot read recovery storage")
		}
	}
	if err := w.check(ctx); err != nil {
		return nil, err
	}
	return data, nil
}

func creationError(err error, message string) error {
	code := 1
	if errors.Is(err, unix.EEXIST) || errors.Is(err, unix.ELOOP) || errors.Is(err, unix.ENOTDIR) {
		code = 2
	}
	return failure(code, message)
}

func referenceBytes(data []byte, reference referenceAsset) bool {
	hash := sha256.Sum256(data)
	return int64(len(data)) == reference.Reference.Bytes && hex.EncodeToString(hash[:]) == reference.Reference.SHA256
}
