package installation

import "context"

// Reserve rollback admission before replacing the committed check evidence.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:668.
func (owner *transaction) prepareRollbackPending(ctx context.Context) error {
	if owner.request.Rollback == "" || owner.pending != nil || !preparedPendingState(owner.record) {
		return conflict("completed rollback preparation grant unavailable")
	}
	if err := publishBytesWith(ctx, owner.tree, ".factory/runtime-publication.pending", nil, 0600, nil, func(ctx context.Context, prepared *Image) error {
		owner.record.PreparedPending = prepared
		return owner.save(ctx)
	}); err != nil {
		return err
	}
	var err error
	owner.pending, err = owner.tree.OpenFile(ctx, ".factory/runtime-publication.pending", 0)
	if err != nil {
		return err
	}
	actual, _, err := observe(ctx, owner.tree, ".factory/runtime-publication.pending", 0)
	if err != nil {
		return err
	}
	if err := owner.pending.Check(ctx); err != nil {
		return err
	}
	if err := owner.finalizePreparedPending(ctx, actual); err != nil {
		return err
	}
	return owner.pending.Check(ctx)
}

func preparedPendingState(record journal) bool {
	return record.Phase == "committed" && record.Direction == "forward" && record.Outcome == "completed" && record.Pending != nil
}

func validPendingObservation(image *Image) bool {
	return image != nil && image.Type == "regular" && image.Mode == 0100600 && image.Bytes == 0 && image.Identity != nil
}

// Only the recorded pre-rename marker permits the existing prepared-object
// comparison; an ordinary observed marker retains strict identity equality.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:680.
func terminalPendingEqual(record journal, actual *Image) bool {
	if record.PreparedPending != nil {
		return preparedPendingState(record) && preparedEqual(actual, record.PreparedPending)
	}
	return imageEqual(actual, record.Pending, true)
}

// Terminal recovery adopts a matching published marker before removing it, or
// clears only unused preparation after freshly confirming marker absence.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:688.
func (owner *transaction) finalizePreparedPending(ctx context.Context, actual *Image) error {
	if owner.record.PreparedPending == nil {
		return nil
	}
	if !preparedPendingState(owner.record) || actual != nil && !terminalPendingEqual(owner.record, actual) {
		return conflict("prepared pending observation differs from current marker")
	}
	current, _, err := observe(ctx, owner.tree, ".factory/runtime-publication.pending", 0)
	if err != nil {
		return err
	}
	if !imageEqual(current, actual, true) {
		return conflict("prepared pending marker changed before observation was recorded")
	}
	if actual != nil {
		owner.record.Pending = actual
	}
	owner.record.PreparedPending = nil
	return owner.save(ctx)
}
