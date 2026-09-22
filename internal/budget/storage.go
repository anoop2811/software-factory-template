package budget

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"syscall"
	"time"
)

type storageOps struct {
	syncFile      func(*os.File) error
	syncDirectory func(*os.File) error
	rename        func(*os.Root, string, string) error
}
type storage struct {
	project, directory         *os.Root
	directoryInfo, historyInfo os.FileInfo
	lock                       *os.File
	lockInfo                   os.FileInfo
	ops                        storageOps
}

func (s *storage) close() {
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
func storageError() error { return errors.New("unsafe or unavailable budget storage") }

// Read never creates storage or repairs modes.
// docs/adr/0069-go-budget-ledger-admission.md:46.
func (l *Ledger) Read(ctx context.Context) (History, error) {
	s, err := l.openStorage(ctx, false)
	if err != nil {
		return History{}, err
	}
	if s == nil {
		return emptyHistory(), nil
	}
	defer s.close()
	return s.read(ctx)
}
func (l *Ledger) openStorage(ctx context.Context, create bool) (result *storage, returned error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s := &storage{ops: l.ops}
	defer func() {
		if result == nil {
			s.close()
		}
	}()
	var err error
	s.project, err = os.OpenRoot(l.root)
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
func (s *storage) check(mutate bool) error {
	info, err := s.project.Lstat(".factory")
	if err != nil || !info.IsDir() || !os.SameFile(info, s.directoryInfo) {
		return storageError()
	}
	if mutate && !owned(info) {
		return storageError()
	}
	for _, name := range []string{"budget.json", "budget.lock"} {
		current, err := s.directory.Lstat(name)
		if errors.Is(err, os.ErrNotExist) {
			if name == "budget.lock" && s.lockInfo != nil {
				return storageError()
			}
			continue
		}
		if err != nil || !regular(current) || mutate && !owned(current) {
			return storageError()
		}
		if name == "budget.lock" && s.lockInfo != nil && !os.SameFile(current, s.lockInfo) {
			return storageError()
		}
	}
	return nil
}
func (s *storage) read(ctx context.Context) (History, error) {
	if err := ctx.Err(); err != nil {
		return History{}, err
	}
	if err := s.check(false); err != nil {
		return History{}, err
	}
	before, err := s.directory.Lstat("budget.json")
	if errors.Is(err, os.ErrNotExist) {
		s.historyInfo = nil
		return emptyHistory(), nil
	}
	if err != nil || !regular(before) {
		return History{}, storageError()
	}
	file, err := s.directory.OpenFile("budget.json", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return History{}, storageError()
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !regular(opened) || !os.SameFile(before, opened) || opened.Size() > historyLimit {
		return History{}, storageError()
	}
	history, err := ParseHistory(ctx, file)
	if err != nil {
		return History{}, err
	}
	after, err := file.Stat()
	if err != nil || !regular(after) || !os.SameFile(opened, after) || opened.Size() != after.Size() || !opened.ModTime().Equal(after.ModTime()) {
		return History{}, storageError()
	}
	s.historyInfo = after
	return history, nil
}

// This is the persistent Python flock inode, never a replaceable lock token.
// docs/adr/0069-go-budget-ledger-admission.md:60.
func (l *Ledger) locked(ctx context.Context) (*storage, error) {
	s, err := l.openStorage(ctx, true)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*storage, error) { s.close(); return nil, err }
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	s.lock, err = s.directory.OpenFile("budget.lock", os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
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
	return s, nil
}
func (s *storage) write(ctx context.Context, h History, id string, pid *int) (returned error) {
	if err := validateHistory(ctx, h); err != nil {
		return err
	}
	data, err := json.Marshal(h.data)
	if err != nil || len(data)+1 > historyLimit {
		return errors.New("budget history exceeds its encoded limit")
	}
	data = append(data, '\n')
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.check(true); err != nil {
		return err
	}
	name := "budget-" + rand.Text() + ".tmp"
	file, err := s.directory.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return &PublicationError{RunID: id, ProcessPID: copyPID(pid)}
	}
	created, statErr := file.Stat()
	committed := false
	publicationError := func() error {
		return &PublicationError{RunID: id, ProcessPID: copyPID(pid), MayHaveCommitted: committed}
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
	if s.ops.syncFile != nil {
		err = s.ops.syncFile(file)
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
	current, err := s.directory.Lstat("budget.json")
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
	if s.ops.rename != nil {
		err = s.ops.rename(s.directory, name, "budget.json")
	} else {
		err = s.directory.Rename(name, "budget.json")
	}
	if err != nil {
		return publicationError()
	}
	committed = true
	directory, err := s.directory.Open(".")
	if err != nil {
		return publicationError()
	}
	if s.ops.syncDirectory != nil {
		err = s.ops.syncDirectory(directory)
	} else {
		err = directory.Sync()
	}
	closeErr := directory.Close()
	if err != nil || closeErr != nil || ctx.Err() != nil {
		return publicationError()
	}
	return nil
}
func copyPID(pid *int) *int {
	if pid == nil {
		return nil
	}
	copied := *pid
	return &copied
}
