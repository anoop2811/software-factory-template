// Package transition excludes runtime transitions across cooperating controllers.
package transition

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

const lockName = "runtime-transition.lock"
const activityName = "runtime-activity"
const pendingName = "runtime-publication.pending"

// Per-operation collaborators qualify actual storage and ownership failures.
type ops struct {
	open          func(context.Context, *os.File, string, int, uint32) (*os.File, error)
	named         func(context.Context, *os.File, string) (unix.Stat_t, error)
	read          func(context.Context, *os.File, []byte) (int, error)
	readDir       func(context.Context, *os.File, int) ([]os.DirEntry, error)
	syncFile      func(context.Context, *os.File) error
	syncDirectory func(context.Context, *os.File) error
	flock         func(context.Context, *os.File, int) error
	unlink        func(context.Context, *os.File, string) error
	close         func(*os.File) error
}

type pin struct {
	parent *os.File
	file   *os.File
	name   string
	stat   unix.Stat_t
	kind   string
}

// Guard owns a permanent flock and, for shared work, durable activity evidence.
// Exclusive acquisition excludes cooperating controllers only; it grants no
// installation or recovery authority. docs/adr/0093-runtime-transition-guard.md:93.
type Guard struct {
	operations   ops
	pins         []*pin
	lock         *pin
	activity     *pin
	marker       *pin
	closed       bool
	recoveryOnly bool
	rootOnly     bool
}

// RecoveryError preserves classification beneath a fixed safe diagnostic.
// docs/adr/0096-interrupted-publication-recovery.md:218.
type RecoveryError struct {
	cause    error
	conflict bool
}

func (e *RecoveryError) Error() string  { return "interrupted publication exclusion unavailable" }
func (e *RecoveryError) Unwrap() error  { return e.cause }
func (e *RecoveryError) Conflict() bool { return e.conflict }

func (g *Guard) issue(cause error, conflict bool) error {
	if g.rootOnly {
		return &RootError{cause: cause, conflict: conflict}
	}
	if g.recoveryOnly {
		return &RecoveryError{cause: cause, conflict: conflict}
	}
	return storageError()
}

func storageError() error { return errors.New("unsafe or unavailable runtime transition storage") }

// Shared records activity before any execution child starts.
// docs/adr/0093-runtime-transition-guard.md:53.
func Shared(ctx context.Context, root string) (*Guard, error) {
	return acquire(ctx, root, false, ops{})
}

// Exclusive refuses immediately if any participant or activity evidence remains.
// docs/adr/0093-runtime-transition-guard.md:88.
func Exclusive(ctx context.Context, root string) (*Guard, error) {
	return acquire(ctx, root, true, ops{})
}

// ExclusiveRecovery excludes cooperating work using existing controls only.
// docs/adr/0096-interrupted-publication-recovery.md:105.
func ExclusiveRecovery(ctx context.Context, root string) (*Guard, error) {
	return acquireRecovery(ctx, root, ops{})
}

// ExclusivePreparation retains existing controls only and requires pending absence.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:418.
func ExclusivePreparation(ctx context.Context, root string) (*Guard, error) {
	return acquireRecoveryFor(ctx, root, ops{}, false)
}
func acquireRecovery(ctx context.Context, root string, operations ops) (*Guard, error) {
	return acquireRecoveryFor(ctx, root, operations, true)
}
func acquireRecoveryFor(ctx context.Context, root string, operations ops, requirePending bool) (result *Guard, returned error) {
	root, err := selectedGuardRoot(root)
	if err != nil {
		return nil, &RecoveryError{conflict: true}
	}
	g := &Guard{operations: operations, recoveryOnly: true}
	defer func() {
		if result == nil {
			returned = errors.Join(g.closeFiles(), returned)
		}
	}()
	project, err := g.existing(ctx, nil, root, "directory")
	if err != nil {
		return nil, err
	}
	state, err := g.existing(ctx, project.file, ".factory", "directory")
	if err != nil {
		return nil, err
	}
	g.lock, err = g.existing(ctx, state.file, lockName, "control")
	if err != nil {
		return nil, err
	}
	g.activity, err = g.existing(ctx, state.file, activityName, "private")
	if err != nil {
		return nil, err
	}
	if err := g.flock(ctx, true); err != nil {
		return nil, err
	}
	read := g.operations.readDir
	if read == nil {
		read = func(_ context.Context, file *os.File, n int) ([]os.DirEntry, error) { return file.ReadDir(n) }
	}
	entries, err := read(ctx, g.activity.file, 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, g.issue(err, false)
	}
	if len(entries) != 0 {
		return nil, g.issue(nil, true)
	}
	if !requirePending {
		_, err := g.named(ctx, state.file, pendingName)
		if !errors.Is(err, unix.ENOENT) {
			return nil, g.issue(err, err == nil)
		}
		if err := g.check(ctx); err != nil {
			return nil, err
		}
		return g, nil
	}
	// Pending is qualified now, but assessment separately owns its later removal.
	// docs/adr/0096-interrupted-publication-recovery.md:115.
	pending, err := g.existing(ctx, state.file, pendingName, "control")
	if err != nil {
		return nil, err
	}
	if err := g.closeFile(pending.file); err != nil {
		return nil, g.issue(err, false)
	}
	g.pins = g.pins[:len(g.pins)-1]
	current, err := g.named(ctx, state.file, pendingName)
	if err != nil {
		return nil, g.issue(err, false)
	}
	if !safe(current, "control") || !same(current, pending.stat) || current.Mode != pending.stat.Mode || current.Gid != pending.stat.Gid || current.Mtim != pending.stat.Mtim || current.Ctim != pending.stat.Ctim {
		return nil, g.issue(nil, true)
	}
	if err := g.check(ctx); err != nil {
		return nil, err
	}
	return g, nil
}

// Check revalidates the retained owner's existing guard, without acquiring anew.
// docs/adr/0094-live-publication-restoration.md:63.
func (g *Guard) Check(ctx context.Context) error {
	if g == nil || g.closed {
		return storageError()
	}
	return g.check(ctx)
}

func (g *Guard) pendingAbsent(ctx context.Context, state *pin) error {
	_, err := g.named(ctx, state.file, pendingName)
	if errors.Is(err, unix.ENOENT) {
		return ctx.Err()
	}
	return errors.Join(errors.New("runtime publication is pending or cannot be inspected"), ctx.Err())
}

func acquire(ctx context.Context, root string, exclusive bool, operations ops) (result *Guard, returned error) {
	root, err := selectedGuardRoot(root)
	if err != nil {
		return nil, err
	}
	g := &Guard{operations: operations}
	defer func() {
		if result == nil {
			returned = errors.Join(returned, g.closeFiles())
		}
	}()
	project, err := g.existing(ctx, nil, root, "directory")
	if err != nil {
		return nil, err
	}
	state, err := g.directory(ctx, project, ".factory", false)
	if err != nil {
		return nil, err
	}
	g.lock, err = g.control(ctx, state, lockName, true)
	if err != nil {
		return nil, err
	}
	if err := g.flock(ctx, exclusive); err != nil {
		return nil, err
	}
	// Any pending entry blocks fresh work while the permanent flock is held.
	// docs/adr/0094-live-publication-restoration.md:59.
	if err := g.pendingAbsent(ctx, state); err != nil {
		return nil, err
	}
	g.activity, err = g.directory(ctx, state, activityName, true)
	if err != nil {
		return nil, err
	}
	if exclusive {
		read := g.operations.readDir
		if read == nil {
			read = func(_ context.Context, file *os.File, n int) ([]os.DirEntry, error) { return file.ReadDir(n) }
		}
		entries, err := read(ctx, g.activity.file, 1)
		if len(entries) != 0 || err != nil && !errors.Is(err, io.EOF) {
			return nil, errors.New("runtime activity remains or cannot be inspected")
		}
	} else {
		g.marker, err = g.control(ctx, g.activity, "activity-"+rand.Text(), false)
		if err != nil {
			return nil, err
		}
		if err := g.sync(ctx, g.marker.file, false); err != nil {
			return nil, err
		}
	}
	// Re-sync existing infrastructure too: visibility after a failed earlier sync
	// does not establish durability. docs/adr/0093-runtime-transition-guard.md:55.
	if err := g.sync(ctx, g.lock.file, false); err != nil {
		return nil, err
	}
	for _, directory := range []*pin{g.activity, state, project} {
		if err := g.sync(ctx, directory.file, true); err != nil {
			return nil, err
		}
	}
	if err := g.check(ctx); err != nil {
		return nil, err
	}
	if err := g.pendingAbsent(ctx, state); err != nil {
		return nil, err
	}
	return g, nil
}

func selectedGuardRoot(root string) (string, error) {
	if root == "" || strings.ContainsRune(root, 0) {
		return "", storageError()
	}
	for _, part := range strings.Split(root, "/") {
		if part == ".." {
			return "", storageError()
		}
	}
	for root != "/" && (strings.HasSuffix(root, "/") || strings.HasSuffix(root, "/.")) {
		if strings.HasSuffix(root, "/.") {
			root = strings.TrimSuffix(root, "/.")
		} else {
			root = strings.TrimSuffix(root, "/")
		}
	}
	return root, nil
}

func (g *Guard) flock(ctx context.Context, exclusive bool) error {
	operation := unix.LOCK_SH | unix.LOCK_NB
	if exclusive {
		operation = unix.LOCK_EX | unix.LOCK_NB
	}
	if err := g.check(ctx); err != nil {
		return err
	}
	var err error
	if g.operations.flock != nil {
		err = g.operations.flock(ctx, g.lock.file, operation)
	} else {
		err = unix.Flock(int(g.lock.file.Fd()), operation)
	}
	if err != nil {
		if g.recoveryOnly || g.rootOnly {
			return g.issue(err, errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN))
		}
		return errors.New("runtime transition is locked or unavailable")
	}
	return g.check(ctx)
}

func safe(stat unix.Stat_t, kind string) bool {
	if int64(stat.Uid) != int64(os.Geteuid()) {
		return false
	}
	if kind == "control" {
		return stat.Mode&unix.S_IFMT == unix.S_IFREG && stat.Mode&07777 == 0600 && stat.Nlink == 1 && stat.Size == 0
	}
	return stat.Mode&unix.S_IFMT == unix.S_IFDIR && stat.Mode&07022 == 0 && (kind != "private" || stat.Mode&07777 == 0700)
}

func same(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode&unix.S_IFMT == b.Mode&unix.S_IFMT
}

// Empty activity evidence stays unchanged across pending qualification and grant.
// docs/adr/0096-interrupted-publication-recovery.md:119.
func unchangedActivity(a, b unix.Stat_t) bool {
	return same(a, b) && a.Mode == b.Mode && a.Uid == b.Uid && a.Gid == b.Gid &&
		a.Nlink == b.Nlink && a.Size == b.Size && a.Mtim == b.Mtim && a.Ctim == b.Ctim
}

func (g *Guard) named(ctx context.Context, parent *os.File, name string) (unix.Stat_t, error) {
	if err := ctx.Err(); err != nil {
		return unix.Stat_t{}, err
	}
	if g.operations.named != nil {
		return g.operations.named(ctx, parent, name)
	}
	fd := unix.AT_FDCWD
	if parent != nil {
		fd = int(parent.Fd())
	}
	var stat unix.Stat_t
	err := unix.Fstatat(fd, name, &stat, unix.AT_SYMLINK_NOFOLLOW)
	return stat, err
}

func (g *Guard) open(ctx context.Context, parent *os.File, name string, flags int, mode uint32) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	flags |= unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
	if g.operations.open != nil {
		return g.operations.open(ctx, parent, name, flags, mode)
	}
	fd := unix.AT_FDCWD
	if parent != nil {
		fd = int(parent.Fd())
	}
	descriptor, err := unix.Openat(fd, name, flags, mode)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(descriptor), "runtime-transition"), nil
}

func (g *Guard) existing(ctx context.Context, parent *os.File, name, kind string) (*pin, error) {
	before, err := g.named(ctx, parent, name)
	if err != nil {
		return nil, errors.Join(g.issue(err, false), ctx.Err())
	}
	if !safe(before, kind) {
		return nil, errors.Join(g.issue(nil, true), ctx.Err())
	}
	flags := unix.O_RDONLY | unix.O_DIRECTORY
	if kind == "control" {
		flags = unix.O_RDWR
	}
	file, err := g.open(ctx, parent, name, flags, 0)
	if err != nil {
		return nil, errors.Join(g.issue(err, false), ctx.Err())
	}
	entry := &pin{parent: parent, file: file, name: name, stat: before, kind: kind}
	g.pins = append(g.pins, entry)
	if err := g.check(ctx); err != nil {
		return nil, err
	}
	return entry, nil
}

func (g *Guard) directory(ctx context.Context, parent *pin, name string, private bool) (*pin, error) {
	if err := g.check(ctx); err != nil {
		return nil, err
	}
	_, err := g.named(ctx, parent.file, name)
	if errors.Is(err, unix.ENOENT) {
		if err := unix.Mkdirat(int(parent.file.Fd()), name, 0700); err != nil && !errors.Is(err, unix.EEXIST) {
			return nil, storageError()
		}
	} else if err != nil {
		return nil, errors.Join(storageError(), ctx.Err())
	}
	kind := "directory"
	if private {
		kind = "private"
	}
	return g.existing(ctx, parent.file, name, kind)
}

func (g *Guard) control(ctx context.Context, parent *pin, name string, reuse bool) (*pin, error) {
	if err := g.check(ctx); err != nil {
		return nil, err
	}
	file, err := g.open(ctx, parent.file, name, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL, 0600)
	if errors.Is(err, unix.EEXIST) && reuse {
		return g.existing(ctx, parent.file, name, "control")
	}
	if err != nil {
		return nil, errors.Join(storageError(), ctx.Err())
	}
	var stat unix.Stat_t
	err = unix.Fstat(int(file.Fd()), &stat)
	entry := &pin{parent: parent.file, file: file, name: name, stat: stat, kind: "control"}
	g.pins = append(g.pins, entry)
	if err != nil || !safe(stat, "control") {
		return nil, storageError()
	}
	if err := g.check(ctx); err != nil {
		return nil, err
	}
	return entry, nil
}

func (g *Guard) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, entry := range g.pins {
		current, err := g.named(ctx, entry.parent, entry.name)
		if err != nil {
			return errors.Join(g.issue(err, false), ctx.Err())
		}
		if !safe(current, entry.kind) || !same(current, entry.stat) {
			return errors.Join(g.issue(nil, true), ctx.Err())
		}
		if g.recoveryOnly && entry == g.activity && !unchangedActivity(entry.stat, current) {
			return g.issue(nil, true)
		}
		if entry.file == nil {
			continue
		}
		var descriptor unix.Stat_t
		if err := unix.Fstat(int(entry.file.Fd()), &descriptor); err != nil {
			return g.issue(err, false)
		}
		if !safe(descriptor, entry.kind) || !same(current, descriptor) {
			return g.issue(nil, true)
		}
		if g.recoveryOnly && entry == g.activity && !unchangedActivity(entry.stat, descriptor) {
			return g.issue(nil, true)
		}
		if entry.kind == "control" {
			read := g.operations.read
			if read == nil {
				read = func(_ context.Context, file *os.File, data []byte) (int, error) { return file.Read(data) }
			}
			var buffer [1]byte
			n, err := read(ctx, entry.file, buffer[:])
			if n != 0 {
				return errors.Join(g.issue(nil, true), ctx.Err())
			}
			if !errors.Is(err, io.EOF) {
				return errors.Join(g.issue(err, false), ctx.Err())
			}
		}
	}
	return ctx.Err()
}

func (g *Guard) sync(ctx context.Context, file *os.File, directory bool) error {
	if err := g.check(ctx); err != nil {
		return err
	}
	operation := g.operations.syncFile
	if directory {
		operation = g.operations.syncDirectory
	}
	var err error
	if operation != nil {
		err = operation(ctx, file)
	} else {
		err = file.Sync()
	}
	if err != nil {
		return errors.Join(errors.New("cannot sync runtime transition storage"), ctx.Err())
	}
	return g.check(ctx)
}

func (g *Guard) closeFile(file *os.File) error {
	if g.operations.close != nil {
		return g.operations.close(file)
	}
	return file.Close()
}

func (g *Guard) closeFiles() error {
	g.closed = true
	var failed bool
	var closeErrors []error
	for i := len(g.pins) - 1; i >= 0; i-- {
		entry := g.pins[i]
		if entry.file != nil {
			if err := g.closeFile(entry.file); err != nil {
				failed = true
				closeErrors = append(closeErrors, err)
			}
			entry.file = nil
		}
	}
	if failed {
		if g.recoveryOnly || g.rootOnly {
			return g.issue(errors.Join(closeErrors...), false)
		}
		return errors.New("cannot close runtime transition storage")
	}
	return nil
}

// Close removes only qualified owned evidence when the caller has already
// established harmless refusal or durable terminal ownership. Otherwise the
// marker remains even though flock is released. docs/adr/0093-runtime-transition-guard.md:79.
func (g *Guard) Close(ctx context.Context, clean bool) (returned error) {
	if g == nil || g.closed {
		return storageError()
	}
	defer func() { returned = errors.Join(returned, g.closeFiles()) }()
	if err := g.check(ctx); err != nil {
		return err
	}
	if !clean || g.marker == nil {
		return nil
	}
	err := g.closeFile(g.marker.file)
	g.marker.file = nil
	if err != nil {
		return errors.New("cannot close runtime activity marker")
	}
	if err := g.check(ctx); err != nil {
		return err
	}
	if g.operations.unlink != nil {
		err = g.operations.unlink(ctx, g.activity.file, g.marker.name)
	} else {
		err = unix.Unlinkat(int(g.activity.file.Fd()), g.marker.name, 0)
	}
	if err != nil {
		return errors.Join(errors.New("cannot remove runtime activity marker"), ctx.Err())
	}
	// The removed marker is no longer an expected pathname. Failed directory
	// sync reports uncertain removal without recreating evidence.
	// docs/adr/0093-runtime-transition-guard.md:83.
	g.pins = g.pins[:len(g.pins)-1]
	return g.sync(ctx, g.activity.file, true)
}
