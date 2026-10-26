package installation

import (
	"context"

	"github.com/anoop2811/software-factory-template/internal/installedlayout"
)

// Completed recovery repeats observation and consent, without republishing the
// installation or rewriting its historical pending receipt.
// docs/DECISION_LOG.md:4848.
func (owner *transaction) completeRecoveryReplay(ctx context.Context) error {
	if !owner.terminalCleanup || !owner.planned.completedRecovery || owner.pending != nil || owner.request.Direction != owner.record.Direction {
		return conflict("completed recovery replay grant unavailable")
	}
	if err := owner.check(ctx); err != nil {
		return err
	}
	current, _, err := observe(ctx, owner.tree, journalPath(owner.record.MigrationID), journalLimit)
	if err != nil {
		return err
	}
	if !imageEqual(current, owner.journalImage, true) {
		return conflict("completed recovery journal changed after consent")
	}
	fresh, err := buildTransitionPlan(ctx, owner.root, owner.request, owner.record, owner.record.Direction == "reverse")
	if err != nil {
		return err
	}
	if !fresh.completedRecovery || fresh.proposal.ProposalDigest != owner.request.Confirmation || len(fresh.proposal.Blockers) != 0 || len(fresh.proposal.RollbackBlockers) != 0 {
		return conflict("completed recovery changed; request a fresh preview")
	}
	for _, action := range fresh.proposal.Actions {
		if action.Action != "retain" {
			return conflict("completed recovery would change installation entries")
		}
	}
	if err := owner.validateCompletedRecovery(ctx); err != nil {
		return err
	}
	owner.completed = true
	return owner.check(ctx)
}

func (owner *transaction) validateCompletedRecovery(ctx context.Context) error {
	if owner.record.Direction == "reverse" {
		if err := owner.validateLegacy(ctx); err != nil {
			return err
		}
		if owner.request.Profile != "go-hybrid-v1" {
			return nil
		}
		_, err := installedlayout.Validate(ctx, owner.tree, nil)
		return err
	}
	_, err := installedlayout.Validate(ctx, owner.tree, []string{owner.request.Source.Version, owner.request.Source.Revision, owner.planned.image.Runtime.SHA256})
	return err
}
