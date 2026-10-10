package installation

import (
	"context"
	"errors"
)

// Resource closure never turns an unconfirmed candidate group into ordinary readiness.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:489.
func (owner *transaction) unresolvedWork() bool {
	if owner.ownershipErr != nil {
		return true
	}
	for _, check := range owner.record.Checks {
		if check.OwnershipUnconfirmed || check.Phase == "checking" {
			return true
		}
	}
	return false
}
func (owner *transaction) persistUnresolved(ctx context.Context) error {
	primary := owner.ownershipErr
	if primary == nil {
		primary = errors.New("installation candidate group ownership remains unconfirmed")
	}
	if owner.tree == nil || owner.guard == nil || owner.lease == nil {
		return &Uncertainty{Operation: "unresolved work; maintenance must remain in force", cause: primary}
	}
	pending, _, err := observe(ctx, owner.tree, ".factory/runtime-publication.pending", 0)
	if err != nil {
		return &Uncertainty{Operation: "unresolved admission evidence; maintenance must remain in force", cause: errors.Join(primary, err)}
	}
	if pending == nil {
		if err := publishBytes(ctx, owner.tree, ".factory/runtime-publication.pending", nil, 0600, nil); err != nil {
			return &Uncertainty{Operation: "unresolved admission publication; maintenance must remain in force", cause: errors.Join(primary, err)}
		}
		pending, _, err = observe(ctx, owner.tree, ".factory/runtime-publication.pending", 0)
		if err != nil {
			return &Uncertainty{Operation: "unresolved admission observation; maintenance must remain in force", cause: errors.Join(primary, err)}
		}
	}
	// Existing pending remains a barrier; its presence never grants new ownership.
	if owner.record.Pending == nil {
		owner.record.Pending = pending
	}
	if owner.journalImage != nil {
		if err := owner.save(ctx); err != nil {
			return &Uncertainty{Operation: "unresolved journal durability; maintenance must remain in force", cause: errors.Join(primary, err)}
		}
	}
	return &Uncertainty{Operation: "unresolved candidate ownership", cause: primary}
}
