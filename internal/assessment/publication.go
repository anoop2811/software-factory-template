package assessment

import (
	"context"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/anoop2811/software-factory-template/internal/filepublish"
	"github.com/anoop2811/software-factory-template/internal/transition"
)

// PublicationRequest selects one existing checked backup and fresh consent.
// docs/adr/0094-live-publication-restoration.md:24.
type PublicationRequest struct {
	MigrationID  string
	Path         string
	Confirmation string
	Replacement  []byte
}

// Publication is an opaque live capability; it has no reconstruction API.
// Value aliases share one private lifecycle and resource ownership.
// docs/adr/0094-live-publication-restoration.md:197.
type Publication struct {
	state *publicationState
}

type publicationState struct {
	mu             sync.Mutex
	w              *recoveryWriter
	guard          *transition.Guard
	operations     publicationOps
	directory      *os.Root
	factory        *writePin
	parent         *writePin
	current        *writePin
	saved          *writePin
	pending        *writePin
	name           string
	original       []byte
	replacement    []byte
	expected       []byte
	mode           os.FileMode
	stages         []*filepublish.Stage
	detached       []*os.File
	candidate      *publicationCandidate
	applyUsed      bool
	changed        bool
	afterPublished bool
	undoPublished  bool
	pendingClosed  bool
	pendingRemoved bool
	finished       bool
	closed         bool
	lastError      error
}

type publicationCandidate struct {
	pin       *writePin
	data      []byte
	restoring bool
}

// PublicationError preserves uncertainty after an attempted active rename.
// docs/adr/0094-live-publication-restoration.md:80.
type PublicationError struct {
	MayHaveChanged bool
	cause          error
}

func (e *PublicationError) Error() string {
	return "live publication failed; inspect retained evidence"
}
func (e *PublicationError) Unwrap() error { return e.cause }

type publicationOps struct {
	storage      recoveryWriteOps
	observe      ops
	prepare      func(context.Context, *os.Root, string, []byte, os.FileMode) (*filepublish.Stage, error)
	openPrepared func(context.Context, *filepublish.Stage) (*os.File, error)
	rename       func(context.Context, *filepublish.Stage, string) error
	unlink       func(context.Context, *os.File, string) error
	close        func(*os.File) error
}

// BeginPublication consumes an existing set; it never creates backups or probes.
// docs/adr/0094-live-publication-restoration.md:163.
func BeginPublication(ctx context.Context, root string, request PublicationRequest) (*Publication, error) {
	return beginPublication(ctx, root, request, publicationOps{})
}

func (p *publicationState) failed(err error) error {
	if err == nil {
		return nil
	}
	if p.changed {
		err = &PublicationError{MayHaveChanged: true, cause: err}
	}
	p.lastError = err
	return err
}

// Apply publishes once; errors never trigger an implicit reversal.
// docs/adr/0094-live-publication-restoration.md:82.
func (p *Publication) Apply(ctx context.Context) error {
	if p == nil || p.state == nil {
		return failure(2, "publication handle unavailable")
	}
	return p.state.apply(ctx)
}

func (p *publicationState) apply(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.finished || p.applyUsed {
		return failure(2, "publication cannot be applied again")
	}
	if err := p.check(ctx); err != nil {
		return p.failed(err)
	}
	p.applyUsed = true
	return p.failed(p.publish(ctx, p.replacement, false))
}

// Restore checks live owned identity and bytes before any reverse publication.
// docs/adr/0094-live-publication-restoration.md:87.
func (p *Publication) Restore(ctx context.Context) error {
	if p == nil || p.state == nil {
		return failure(2, "publication handle unavailable")
	}
	return p.state.restore(ctx)
}

func (p *publicationState) restore(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.finished {
		return failure(2, "publication cannot be restored again")
	}
	if err := p.check(ctx); err != nil {
		return p.failed(err)
	}
	if p.afterPublished && !p.undoPublished {
		if err := p.publish(ctx, p.original, true); err != nil {
			return p.failed(err)
		}
	}
	// Abort and retry after our own reverse rename share durability checks.
	// docs/adr/0094-live-publication-restoration.md:96.
	if err := p.check(ctx); err != nil {
		return p.failed(err)
	}
	if err := p.w.sync(ctx, p.current); err != nil {
		return p.failed(err)
	}
	if err := p.w.sync(ctx, p.parent); err != nil {
		return p.failed(err)
	}
	if err := p.removePending(ctx); err != nil {
		return p.failed(err)
	}
	p.finished = true
	p.lastError = nil
	return nil
}

// Close releases resources without restoring or clearing unresolved evidence.
// docs/adr/0094-live-publication-restoration.md:108.
func (p *Publication) Close(ctx context.Context) error {
	if p == nil || p.state == nil {
		return failure(2, "publication handle unavailable")
	}
	return p.state.close(ctx)
}

func (p *publicationState) close(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return failure(2, "publication handle is closed")
	}
	p.closed = true
	var cleanupErrors []error
	for _, stage := range p.stages {
		if stage.Cleanup() != nil {
			cleanupErrors = append(cleanupErrors, failure(1, "cannot clean publication staging"))
		}
	}
	if p.directory != nil && p.directory.Close() != nil {
		cleanupErrors = append(cleanupErrors, failure(1, "cannot close publication directory"))
	}
	if p.w != nil {
		if p.operations.close != nil {
			p.w.ops.close = p.operations.close
		}
		for _, file := range p.detached {
			if p.w.closeFile(file) != nil {
				cleanupErrors = append(cleanupErrors, failure(1, "cannot close publication input"))
			}
		}
		cleanupErrors = append(cleanupErrors, p.w.close())
	}
	if p.guard != nil {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if p.guard.Close(cleanup, false) != nil {
			cleanupErrors = append(cleanupErrors, failure(1, "cannot close publication transition guard"))
		}
	}
	if p.pending != nil && !p.finished {
		incomplete := failure(1, "publication incomplete; inspect local evidence")
		if p.changed {
			incomplete = &PublicationError{MayHaveChanged: true, cause: incomplete}
		}
		cleanupErrors = append(cleanupErrors, incomplete, p.lastError)
	}
	cleanupErrors = append(cleanupErrors, ctx.Err())
	return errors.Join(cleanupErrors...)
}
