package installation

import (
	"context"
	"errors"
	"github.com/anoop2811/software-factory-template/internal/native"
	"time"
)

// Per-operation collaboration reports actual native work without exposing a shipping bypass.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:483.
type commandOps struct {
	executeObserved func(context.Context, string, []string, map[string]string, time.Duration, func(context.Context, int) error) (native.CommandResult, error)
}

func (owner *transaction) executeObserved(ctx context.Context, root string, argv []string, environment map[string]string, allowance time.Duration, onSpawn func(context.Context, int) error) (native.CommandResult, error) {
	execute := owner.operations.executeObserved
	if execute == nil {
		execute = native.ExecuteCommandObserved
	}
	result, err := execute(ctx, root, argv, environment, allowance, onSpawn)
	if result.OwnershipUnconfirmed {
		owner.ownershipErr = errors.Join(owner.ownershipErr, err, &native.OwnershipError{ProcessPID: result.ProcessPID})
	}
	return result, err
}
