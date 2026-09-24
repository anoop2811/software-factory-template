package budget

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/anoop2811/software-factory-template/internal/statefile"
)

type storageOps struct {
	openLock      func(*os.Root, int) (*os.File, error)
	openHistory   func(*os.Root) (*os.File, error)
	syncFile      func(*os.File) error
	syncDirectory func(*os.File) error
	rename        func(*os.Root, string, string) error
}
type storage struct{ file *statefile.File }

func (s *storage) close() { s.file.Close() }
func storageError() error { return errors.New("unsafe or unavailable budget storage") }
func storageFailure(err error) error {
	var unsafe statefile.UnsafeError
	if errors.As(err, &unsafe) {
		return storageError()
	}
	return err
}

// Shared filesystem mechanics preserve the budget's blocking lock and parser.
// docs/adr/0074-go-loop-checkpoint-storage.md:51.
func (l *Ledger) openStorage(ctx context.Context, create bool) (*storage, error) {
	f, err := statefile.Open(ctx, l.root, statefile.Names{History: "budget.json", Lock: "budget.lock", Temporary: "budget-"}, create, statefile.Ops{
		OpenLock: l.ops.openLock, OpenHistory: l.ops.openHistory, SyncFile: l.ops.syncFile, SyncDirectory: l.ops.syncDirectory, Rename: l.ops.rename,
	})
	if err != nil || f == nil {
		return nil, storageFailure(err)
	}
	return &storage{file: f}, nil
}
func (l *Ledger) Read(ctx context.Context) (History, error) {
	s, err := l.openStorage(ctx, false)
	if err != nil {
		return History{}, err
	}
	if s == nil {
		return emptyHistory(), nil
	}
	defer s.close()
	for attempt := 0; attempt < 8; attempt++ {
		if err := ctx.Err(); err != nil {
			return History{}, err
		}
		h, err := s.read(ctx)
		var replaced statefile.ReplacedError
		if !errors.As(err, &replaced) {
			return h, err
		}
	}
	return History{}, storageError()
}
func (s *storage) read(ctx context.Context) (History, error) {
	h := emptyHistory()
	err := s.file.Read(ctx, func(ctx context.Context, r io.Reader) error { var err error; h, err = ParseHistory(ctx, r); return err })
	return h, storageFailure(err)
}
func (l *Ledger) locked(ctx context.Context) (*storage, error) {
	s, err := l.openStorage(ctx, true)
	if err != nil {
		return nil, err
	}
	if err := s.file.Acquire(ctx, true); err != nil {
		s.close()
		return nil, storageFailure(err)
	}
	return s, nil
}
func (s *storage) write(ctx context.Context, h History, id string, pid *int) error {
	if err := validateHistory(ctx, h); err != nil {
		return err
	}
	data, err := json.Marshal(h.data)
	if err != nil || len(data)+1 > historyLimit {
		return errors.New("budget history exceeds its encoded limit")
	}
	err = s.file.Publish(ctx, append(data, '\n'))
	var publication *statefile.PublicationError
	if errors.As(err, &publication) {
		return &PublicationError{RunID: id, ProcessPID: copyPID(pid), MayHaveCommitted: publication.MayHaveCommitted}
	}
	return storageFailure(err)
}
func copyPID(pid *int) *int {
	if pid == nil {
		return nil
	}
	copied := *pid
	return &copied
}
