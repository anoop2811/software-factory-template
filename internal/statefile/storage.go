// Package statefile owns the shared persistent state storage mechanics.
package statefile

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"strings"
	"syscall"
	"time"
)

// Names selects fixed basenames within the pinned state directory.
type Names struct{ History, Lock, Temporary string }

// Ops supplies per-instance filesystem collaborators for deterministic faults.
type Ops struct {
	OpenLock      func(*os.Root, int) (*os.File, error)
	OpenHistory   func(*os.Root) (*os.File, error)
	SyncFile      func(*os.File) error
	SyncDirectory func(*os.File) error
	Rename        func(*os.Root, string, string) error
}

// File owns pinned directory handles and an optional persistent lock.
// It is not safe for concurrent method calls; Close releases all owned handles.
type File struct {
	project, directory         *os.Root
	directoryInfo, historyInfo os.FileInfo
	lock                       *os.File
	lockInfo                   os.FileInfo
	ops                        Ops
	names                      Names
	readReady, held, closed    bool
}

// Limit bounds each encoded state document in bytes.
const Limit = 32 << 20

// UnsafeError denotes storage whose ownership or identity cannot be established.
type UnsafeError struct{}

func (UnsafeError) Error() string { return "unsafe or unavailable state storage" }

// BusyError reports immediate nonblocking lock contention.
type BusyError struct{}

func (BusyError) Error() string { return "state storage is locked" }

// PublicationError distinguishes a potentially committed atomic replacement.
type PublicationError struct{ MayHaveCommitted bool }

func (*PublicationError) Error() string { return "state publication failed" }

// Close releases handles and permanently invalidates this instance.
func (s *File) Close() {
	s.closed = true
	s.held = false
	s.readReady = false
	if s.lock != nil {
		_ = s.lock.Close()
	}
	if s.directory != nil {
		_ = s.directory.Close()
	}
	if s.project != nil {
		_ = s.project.Close()
	}
}
func owned(info os.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && int64(st.Uid) == int64(os.Geteuid())
}
func regular(info os.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && info.Mode().IsRegular() && st.Nlink == 1
}
func storageError() error { return UnsafeError{} }

// Open pins existing storage, optionally creating private writable infrastructure.
// docs/adr/0074-go-loop-checkpoint-storage.md:57.
func Open(ctx context.Context, root string, names Names, create bool, ops Ops) (result *File, returned error) {
	for _, name := range []string{names.History, names.Lock, names.Temporary} {
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
			return nil, storageError()
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s := &File{ops: ops, names: names}
	defer func() {
		if result == nil {
			s.Close()
		}
	}()
	var err error
	s.project, err = os.OpenRoot(root)
	if err != nil {
		if !create && errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, storageError()
	}
	info, err := s.project.Lstat(".factory")
	if errors.Is(err, os.ErrNotExist) {
		if !create {
			return nil, nil
		}
		if err := s.project.Mkdir(".factory", 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, storageError()
		}
		info, err = s.project.Lstat(".factory")
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, storageError()
	}
	s.directoryInfo = info
	s.directory, err = s.project.OpenRoot(".factory")
	if err != nil {
		return nil, storageError()
	}
	opened, err := s.directory.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		return nil, storageError()
	}
	if err := s.check(false); err != nil {
		return nil, err
	}
	if create {
		if !owned(opened) {
			return nil, storageError()
		}
		directory, err := s.directory.Open(".")
		if err != nil {
			return nil, storageError()
		}
		chmodErr := directory.Chmod(0700)
		closeErr := directory.Close()
		if chmodErr != nil || closeErr != nil {
			return nil, storageError()
		}
	}
	return s, nil
}
func (s *File) check(mutate bool) error {
	if s.closed || s.project == nil || s.directory == nil {
		return storageError()
	}
	info, err := s.project.Lstat(".factory")
	if err != nil || !info.IsDir() || !os.SameFile(info, s.directoryInfo) {
		return storageError()
	}
	if mutate && !owned(info) {
		return storageError()
	}
	for _, name := range []string{s.names.History, s.names.Lock} {
		current, err := s.directory.Lstat(name)
		if errors.Is(err, os.ErrNotExist) {
			if name == s.names.Lock && s.lockInfo != nil {
				return storageError()
			}
			continue
		}
		if err != nil || !regular(current) || mutate && !owned(current) {
			return storageError()
		}
		if name == s.names.Lock && s.lockInfo != nil && !os.SameFile(current, s.lockInfo) {
			return storageError()
		}
	}
	return nil
}

// Read validates a bounded snapshot around domain parsing and records its identity.
// A missing history does not invoke consume. Locked callers must not retry errors.
func (s *File) Read(ctx context.Context, consume func(context.Context, io.Reader) error) error {
	s.readReady = false
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.check(false); err != nil {
		return err
	}
	before, err := s.directory.Lstat(s.names.History)
	if errors.Is(err, os.ErrNotExist) {
		s.historyInfo = nil
		s.readReady = true
		return nil
	}
	if err != nil || !regular(before) {
		return storageError()
	}
	var file *os.File
	if s.ops.OpenHistory != nil {
		file, err = s.ops.OpenHistory(s.directory)
	} else {
		file, err = s.directory.OpenFile(s.names.History, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	}
	if err != nil {
		return storageError()
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !snapshotDescriptor(opened) || opened.Size() > Limit {
		return storageError()
	}
	if !regular(opened) || !os.SameFile(before, opened) {
		if s.replacedSnapshot(before) {
			return ReplacedError{}
		}
		return storageError()
	}
	err = consume(ctx, file)
	if err != nil {
		return err
	}
	after, err := file.Stat()
	if err != nil || !snapshotDescriptor(after) || !os.SameFile(opened, after) || opened.Size() != after.Size() || !opened.ModTime().Equal(after.ModTime()) {
		return storageError()
	}
	if !regular(after) {
		if s.replacedSnapshot(opened) {
			return ReplacedError{}
		}
		return storageError()
	}
	s.historyInfo = after
	s.readReady = true
	return nil
}

// This is the persistent Python flock inode, never a replaceable lock token.
// docs/adr/0069-go-budget-ledger-admission.md:60.
func (s *File) Acquire(ctx context.Context, wait bool) error {
	if s.closed || s.held || s.project == nil || s.directory == nil {
		return storageError()
	}
	var err error
	fail := func(err error) error { s.Close(); return err }
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	// Exclusive first creation avoids the observed competing O_CREATE failure;
	// only an existing lock permits opening again, never recreation.
	// docs/adr/0070-go-budget-execution-controller.md:167.
	openLock := s.ops.OpenLock
	if openLock == nil {
		openLock = func(directory *os.Root, flags int) (*os.File, error) {
			return directory.OpenFile(s.names.Lock, flags, 0600)
		}
	}
	flags := os.O_RDWR | syscall.O_NOFOLLOW | syscall.O_NONBLOCK
	s.lock, err = openLock(s.directory, flags|os.O_CREATE|os.O_EXCL)
	if errors.Is(err, os.ErrExist) {
		s.lock, err = openLock(s.directory, flags)
	}
	if err != nil {
		return fail(storageError())
	}
	s.lockInfo, err = s.lock.Stat()
	if err != nil || !regular(s.lockInfo) || !owned(s.lockInfo) {
		return fail(storageError())
	}
	if err := s.check(true); err != nil {
		return fail(err)
	}
	if s.lock.Chmod(0600) != nil {
		return fail(storageError())
	}
	for {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		err = syscall.Flock(int(s.lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EINTR) {
			return fail(storageError())
		}
		if !wait {
			return fail(BusyError{})
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fail(ctx.Err())
		case <-timer.C:
		}
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	if err := s.check(true); err != nil {
		return fail(err)
	}
	s.held = true
	return nil
}

// Publish durably replaces the last successfully read snapshot under the held lock.
// Domain validation and bounded encoding belong to the caller.
func (s *File) Publish(ctx context.Context, data []byte) (returned error) {
	if s.closed || !s.held || !s.readReady || len(data) > Limit {
		return &PublicationError{}
	}
	s.readReady = false
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.check(true); err != nil {
		return err
	}
	name := s.names.Temporary + rand.Text() + ".tmp"
	file, err := s.directory.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return &PublicationError{}
	}
	created, statErr := file.Stat()
	committed := false
	publicationError := func() error {
		return &PublicationError{MayHaveCommitted: committed}
	}
	defer func() {
		_ = file.Close()
		current, err := s.directory.Lstat(name)
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		if err != nil || statErr != nil || !os.SameFile(created, current) {
			returned = publicationError()
			return
		}
		if s.directory.Remove(name) != nil {
			returned = publicationError()
		}
	}()
	if statErr != nil || !regular(created) || !owned(created) || file.Chmod(0600) != nil {
		return publicationError()
	}
	written, err := file.Write(data)
	if err != nil || written != len(data) {
		return publicationError()
	}
	if s.ops.SyncFile != nil {
		err = s.ops.SyncFile(file)
	} else {
		err = file.Sync()
	}
	if err != nil {
		return publicationError()
	}
	if file.Close() != nil {
		return publicationError()
	}
	if ctx.Err() != nil {
		return publicationError()
	}
	if s.check(true) != nil {
		return publicationError()
	}
	current, err := s.directory.Lstat(s.names.History)
	if s.historyInfo == nil {
		if !errors.Is(err, os.ErrNotExist) {
			return publicationError()
		}
	} else {
		if err != nil || !os.SameFile(current, s.historyInfo) {
			return publicationError()
		}
	}
	temporary, err := s.directory.Lstat(name)
	if err != nil || !regular(temporary) || !os.SameFile(created, temporary) {
		return publicationError()
	}
	if s.ops.Rename != nil {
		err = s.ops.Rename(s.directory, name, s.names.History)
	} else {
		err = s.directory.Rename(name, s.names.History)
	}
	if err != nil {
		return publicationError()
	}
	committed = true
	directory, err := s.directory.Open(".")
	if err != nil {
		return publicationError()
	}
	if s.ops.SyncDirectory != nil {
		err = s.ops.SyncDirectory(directory)
	} else {
		err = directory.Sync()
	}
	closeErr := directory.Close()
	if err != nil || closeErr != nil || ctx.Err() != nil {
		return publicationError()
	}
	s.historyInfo = created
	s.readReady = true
	return nil
}

// A detached regular descriptor is evidence only when a different safe pathname
// occupant exists. Symlink, hard-link, parse and in-place mutation failures are
// never reclassified as replacement. docs/adr/0070-go-budget-execution-controller.md:146.
// ReplacedError permits an unlocked caller to reacquire a proven replaced snapshot.
type ReplacedError struct{}

func (ReplacedError) Error() string { return "budget snapshot was atomically replaced" }
func snapshotDescriptor(info os.FileInfo) bool {
	metadata, ok := info.Sys().(*syscall.Stat_t)
	return ok && info.Mode().IsRegular() && metadata.Nlink <= 1
}
func (s *File) replacedSnapshot(previous os.FileInfo) bool {
	current, err := s.directory.Lstat(s.names.History)
	return err == nil && regular(current) && !os.SameFile(previous, current)
}
