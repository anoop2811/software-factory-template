package transition

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"sync"
)

// RootLease holds physical directory exclusion without creating state entries.
// Lock order is root, transition, controller stores.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:275.
type RootLease struct{ state *rootLeaseState }
type rootLeaseState struct {
	mu       sync.Mutex
	guard    *Guard
	root     *pin
	factory  *pin
	readOnly bool
	closed   bool
}

func ReadOnly(ctx context.Context, root string) (*RootLease, error) {
	return rootLease(ctx, root, false)
}
func RootExclusive(ctx context.Context, root string) (*RootLease, error) {
	return rootLease(ctx, root, true)
}
func rootLease(ctx context.Context, root string, exclusive bool) (result *RootLease, returned error) {
	selected, err := selectedGuardRoot(root)
	if err != nil {
		return nil, err
	}
	g := &Guard{rootOnly: true}
	defer func() {
		if result == nil {
			returned = errors.Join(returned, g.closeFiles())
		}
	}()
	project, err := g.existing(ctx, nil, selected, "directory")
	if err != nil {
		return nil, err
	}
	g.lock = project
	if err := g.flock(ctx, exclusive); err != nil {
		return nil, err
	}

	state := &rootLeaseState{guard: g, root: project, readOnly: !exclusive}
	if !exclusive {
		_, err := g.named(ctx, project.file, ".factory")
		switch {
		case errors.Is(err, unix.ENOENT):
			state.factory = nil
		case err != nil:
			return nil, g.issue(err, false)
		default:
			state.factory, err = g.existing(ctx, project.file, ".factory", "directory")
			if err != nil {
				return nil, err
			}
		}
	}
	if err := state.check(ctx); err != nil {
		return nil, err
	}
	return &RootLease{state: state}, nil
}
func (state *rootLeaseState) check(ctx context.Context) error {
	if err := state.guard.check(ctx); err != nil {
		return err
	}
	if !state.readOnly {
		return nil
	}
	if state.factory == nil {
		_, err := state.guard.named(ctx, state.root.file, ".factory")
		if !errors.Is(err, unix.ENOENT) {
			return state.guard.issue(err, err == nil)
		}
		return ctx.Err()
	}
	_, err := state.guard.named(ctx, state.factory.file, pendingName)
	if errors.Is(err, unix.ENOENT) {
		return ctx.Err()
	}
	return state.guard.issue(err, err == nil)
}

func (lease *RootLease) Check(ctx context.Context) error {
	if lease == nil || lease.state == nil {
		return storageError()
	}
	state := lease.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.closed {
		return storageError()
	}
	return state.check(ctx)
}
func (lease *RootLease) Close(ctx context.Context) error {
	if lease == nil || lease.state == nil {
		return storageError()
	}
	state := lease.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.closed {
		return storageError()
	}
	state.closed = true
	return errors.Join(state.guard.closeFiles(), ctx.Err())
}

// RootError retains operational causes for the additive physical root lease.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:150.
type RootError struct {
	cause    error
	conflict bool
}

func (e *RootError) Error() string  { return "physical installation root lease unavailable" }
func (e *RootError) Unwrap() error  { return e.cause }
func (e *RootError) Conflict() bool { return e.conflict }
