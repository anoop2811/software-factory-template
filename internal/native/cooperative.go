package native

import (
	"context"
	"errors"
	"syscall"
	"time"
)

// Cooperating Go controllers own their nested native groups. Keep their grace
// inside the caller's cleanup budget, leaving time for escalation and proof.
// docs/adr/0122-default-packaged-go-upgrade.md:155.
func cooperate(ctx context.Context, pid int, waited <-chan error, cleanupLimit time.Duration) (error, bool, error) {
	signalErr := syscall.Kill(-pid, syscall.SIGTERM)
	if errors.Is(signalErr, syscall.ESRCH) {
		signalErr = nil
	}
	if signalErr != nil {
		return nil, false, signalErr
	}
	timer := time.NewTimer(min(2*time.Second, cleanupLimit/2))
	defer timer.Stop()
	select {
	case err := <-waited:
		return err, true, nil
	case <-timer.C:
		return nil, false, nil
	case <-ctx.Done():
		return nil, false, nil
	}
}
