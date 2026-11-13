package qualification

import (
	"context"
	"fmt"
	"time"
)

// RecoveryLimits bounds repository qualification, not the installed CLI.
// docs/adr/0152-phase-bounded-local-recovery-qualification.md:31.
type RecoveryLimits struct {
	Preview, Admission, Publication, Command time.Duration
}

func LocalRecoveryLimits() RecoveryLimits {
	return RecoveryLimits{Preview: 120 * time.Second, Admission: 120 * time.Second, Publication: 60 * time.Second, Command: 180 * time.Second}
}

func (limits RecoveryLimits) Context(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, limits.Command)
}

func (limits RecoveryLimits) PreviewContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, limits.Preview)
}

func (limits RecoveryLimits) AwaitAdmission(ctx context.Context, observe func() (bool, error), exited func() error) error {
	return awaitRecoveryReceipt(ctx, limits.Admission, "publisher admission", observe, exited)
}

func (limits RecoveryLimits) AwaitPublication(ctx context.Context, observe func() (bool, error), exited func() error) error {
	return awaitRecoveryReceipt(ctx, limits.Publication, "durable publication", observe, exited)
}

// Observers are bounded, read-only probes. Every phase inherits the original
// command context; a late receipt cannot replace cancellation with success.
// docs/adr/0152-phase-bounded-local-recovery-qualification.md:45.
func awaitRecoveryReceipt(parent context.Context, allowance time.Duration, phase string, observe func() (bool, error), exited func() error) error {
	ctx, cancel := context.WithTimeout(parent, allowance)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	failure := func(err error) error { return fmt.Errorf("%s: %w", phase, err) }
	for {
		if err := ctx.Err(); err != nil {
			return failure(err)
		}
		if err := exited(); err != nil {
			return failure(err)
		}
		ready, err := observe()
		if err != nil {
			return failure(err)
		}
		if err := ctx.Err(); err != nil {
			return failure(err)
		}
		if ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return failure(ctx.Err())
		case <-ticker.C:
		}
	}
}
