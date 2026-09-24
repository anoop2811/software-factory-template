package loop

import (
	"context"
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

// Store locates checkout checkpoints without accessing the filesystem at construction.
type Store struct {
	root string
	ops  storageOps
}

// Transaction holds the loop lock across reads and durable checkpoint saves.
// It is not safe for concurrent use. Close always releases its owned handles.
type Transaction struct {
	file                 *statefile.File
	closed, failed, read bool
}

// PublicationError records whether a failed save may already have replaced history.
type PublicationError struct{ MayHaveCommitted bool }

func (*PublicationError) Error() string {
	return "loop checkpoint publication failed; inspect before proceeding"
}

// NewStore selects a caller-authorized checkout root.
func NewStore(root string) *Store { return &Store{root: root} }
func (s *Store) open(ctx context.Context, create bool) (*statefile.File, error) {
	return statefile.Open(ctx, s.root, statefile.Names{History: "loops.json", Lock: "loops.lock", Temporary: "loop-"}, create, statefile.Ops{
		OpenLock: s.ops.openLock, OpenHistory: s.ops.openHistory, SyncFile: s.ops.syncFile, SyncDirectory: s.ops.syncDirectory, Rename: s.ops.rename,
	})
}
func readHistory(ctx context.Context, f *statefile.File) (History, error) {
	h := emptyHistory()
	err := f.Read(ctx, func(ctx context.Context, r io.Reader) error { var err error; h, err = ParseHistory(ctx, r); return err })
	return h, err
}

// Read observes validated snapshots without creating or repairing storage.
// docs/adr/0074-go-loop-checkpoint-storage.md:21.
func (s *Store) Read(ctx context.Context) (History, error) {
	f, err := s.open(ctx, false)
	if err != nil {
		return History{}, err
	}
	if f == nil {
		return emptyHistory(), nil
	}
	defer f.Close()
	for attempt := 0; attempt < 8; attempt++ {
		if err := ctx.Err(); err != nil {
			return History{}, err
		}
		h, err := readHistory(ctx, f)
		var replaced statefile.ReplacedError
		if !errors.As(err, &replaced) {
			return h, err
		}
	}
	return History{}, checkpointError()
}

// Lock retains a persistent, nonblocking transaction until explicit Close.
// docs/adr/0074-go-loop-checkpoint-storage.md:65.
func (s *Store) Lock(ctx context.Context) (*Transaction, error) {
	f, err := s.open(ctx, true)
	if err != nil {
		return nil, err
	}
	if err := f.Acquire(ctx, false); err != nil {
		f.Close()
		return nil, err
	}
	return &Transaction{file: f}, nil
}

// Close is idempotent and permanently invalidates the transaction.
func (t *Transaction) Close() error {
	if t == nil || t.closed || t.file == nil {
		return nil
	}
	t.closed = true
	t.file.Close()
	return nil
}

// Read validates current history while retaining the lock, including after failed saves.
func (t *Transaction) Read(ctx context.Context) (History, error) {
	if t == nil || t.closed || t.file == nil {
		return History{}, checkpointError()
	}
	h, err := readHistory(ctx, t.file)
	t.read = err == nil
	return h, err
}

// Write saves validated history after a successful read; failed publication forbids retry.
func (t *Transaction) Write(ctx context.Context, h History) error {
	if t == nil || t.closed || t.file == nil || t.failed || !t.read {
		return checkpointError()
	}
	if err := validateHistory(ctx, h); err != nil {
		return err
	}
	data, err := encodeHistory(ctx, h.data)
	if err != nil || len(data)+1 > statefile.Limit {
		return checkpointError()
	}
	if err := t.file.Publish(ctx, append(data, '\n')); err != nil {
		t.failed = true
		var publication *statefile.PublicationError
		if errors.As(err, &publication) {
			return &PublicationError{MayHaveCommitted: publication.MayHaveCommitted}
		}
		return err
	}
	return nil
}
